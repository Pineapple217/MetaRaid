package spotify

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/Pineapple217/MetaRaid/pkg/config"
	"github.com/Pineapple217/MetaRaid/pkg/helper"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
)

func NewClient(conf config.Spotify) []*spotify.Client {
	ctx := context.Background()
	clients := []*spotify.Client{}
	for _, keys := range conf.Clients {
		config := &clientcredentials.Config{
			ClientID:     keys.ClientId,
			ClientSecret: keys.ClientSecret,
			TokenURL:     spotifyauth.TokenURL,
		}
		token, err := config.Token(ctx)
		if err != nil {
			helper.DieMsg(err, "could not get token")
		}

		httpClient := spotifyauth.New().Client(ctx, token)
		client := spotify.New(httpClient, spotify.WithRetry(true))
		clients = append(clients, client)
	}
	slog.Info("Loaded Spotify api keys", "count", len(clients))
	return clients
}

func GetArtists(fs []*FullerTrack, mainArtist spotify.ID) []spotify.ID {
	seen := make(map[string]struct{})
	out := []spotify.ID{}
	for _, t := range fs {
		for _, a := range t.Track.Artists {
			if a.ID == mainArtist {
				continue
			}
			if _, exists := seen[a.ID.String()]; !exists {
				seen[a.ID.String()] = struct{}{}
				out = append(out, a.ID)
			}

		}
	}
	return out
}

func FetchArtistTracks(client *spotify.Client, ctx context.Context, id spotify.ID) ([]*FullerTrack, int, error) {
	requestCount := 0
	albums, err := client.GetArtistAlbums(
		ctx,
		id,
		[]spotify.AlbumType{
			spotify.AlbumTypeAlbum,
			spotify.AlbumTypeSingle,
			spotify.AlbumTypeCompilation,
			// spotify.AlbumTypeAppearsOn,
		}, spotify.Limit(50))
	if err != nil {
		return nil, requestCount, fmt.Errorf("hier %s", err)
	}
	requestCount++

	allAlbums := []spotify.SimpleAlbum{}
	// cap to prevent infinite loop
	for range 100 {
		allAlbums = append(allAlbums, albums.Albums...)
		err = client.NextPage(ctx, albums)
		if err == spotify.ErrNoMorePages {
			break
		}
		if err != nil {
			return nil, requestCount, err
		}
		requestCount++
	}

	allSimpleTracks := []spotify.SimpleTrack{}
	for chunk := range slices.Chunk(allAlbums, 20) {
		ids := make([]spotify.ID, len(chunk))
		for i, a := range chunk {
			ids[i] = a.ID
		}
		fullAlbums, err := client.GetAlbums(ctx, ids)
		if err != nil {
			return nil, requestCount, err
		}
		requestCount++
		for _, fullAlbum := range fullAlbums {
			tracks := fullAlbum.Tracks
			for range 100 {
				allSimpleTracks = append(allSimpleTracks, tracks.Tracks...)
				err = client.NextPage(ctx, &tracks)
				if err == spotify.ErrNoMorePages {
					break
				}
				if err != nil {
					return nil, 0, err
				}
				requestCount++
			}
		}
	}

	allTracks := make([]*FullerTrack, len(allSimpleTracks))
	offset := 0
	for chunk := range slices.Chunk(allSimpleTracks, 100) {
		ids := make([]spotify.ID, len(chunk))
		for i, a := range chunk {
			ids[i] = a.ID
		}

		f, err := client.GetAudioFeatures(ctx, ids...)
		if err != nil {
			return nil, requestCount, err
		}
		requestCount++

		fullTracks := []*spotify.FullTrack{}
		for subChunk := range slices.Chunk(ids, 50) {
			full, err := client.GetTracks(ctx, subChunk, spotify.Limit(50))
			if err != nil {
				return nil, 0, err
			}
			requestCount++
			fullTracks = append(fullTracks, full...)
		}
		for i := range len(ids) {
			allTracks[i+offset] = &FullerTrack{
				Track:    fullTracks[i],
				Features: f[i],
			}
		}
		offset += 100
	}
	return allTracks, requestCount, nil
}
