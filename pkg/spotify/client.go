package spotify

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/Pineapple217/MetaRaid/pkg/config"
	"github.com/Pineapple217/MetaRaid/pkg/helper"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
)

type Client struct {
	Client   *spotify.Client
	Status   status
	Cooldown time.Duration
	Name     string
}

type status int

const (
	Available status = iota
	Cold
)

func (s status) String() string {
	switch s {
	case Available:
		return "available"
	case Cold:
		return "cold"
	default:
		return "unknown"
	}
}

func NewClient(conf config.Spotify) []*Client {
	ctx := context.Background()
	clients := []*Client{}
	for _, keys := range conf.Clients {
		config := &clientcredentials.Config{
			ClientID:     keys.ClientId,
			ClientSecret: keys.ClientSecret,
			TokenURL:     spotifyauth.TokenURL,
		}
		token, err := config.Token(ctx)
		helper.MaybeDie(err, "could not get token")

		httpClient := spotifyauth.New().Client(ctx, token)
		client := spotify.New(
			httpClient,
			spotify.WithRetry(true),
			spotify.WithMaxRetryDuration(conf.MaxRetryDuration),
		)
		c := Client{
			Client: client,
			Name:   keys.Name,
		}
		err = c.UpdateStatusAuto(ctx)
		helper.MaybeDieErr(err)

		clients = append(clients, &c)
	}
	slog.Info("Loaded Spotify api keys", "count", len(clients))
	return clients
}

func (c *Client) UpdateStatusAuto(ctx context.Context) error {
	_, err := c.Client.GetTrack(ctx, "0VjIjW4GlUZAMYd2vXMi3b")
	var maxErr *spotify.MaxRetryDurationExceededErr
	if err != nil {
		if errors.As(err, &maxErr) {
			c.Cooldown = maxErr.RetryAfter
			c.Status = Cold
			return nil
		}
		return err
	}
	c.Status = Available
	return nil
}

func (c *Client) UpdateStatus(err *spotify.MaxRetryDurationExceededErr) {
	if err == nil {
		return
	}
	c.Cooldown = err.RetryAfter
	c.Status = Cold
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

func getAllArtists(fs *[]spotify.SimpleTrack) []spotify.ID {
	seen := make(map[spotify.ID]struct{})
	out := []spotify.ID{}
	for _, t := range *fs {
		for _, a := range t.Artists {
			if _, exists := seen[a.ID]; !exists {
				seen[a.ID] = struct{}{}
				out = append(out, a.ID)
			}

		}
	}
	return out
}

func (c *Client) FetchArtistTracks(ctx context.Context, id spotify.ID) ([]*FullerTrack, int, error) {
	requestCount := 0
	albums, err := c.Client.GetArtistAlbums(
		ctx,
		id,
		[]spotify.AlbumType{
			spotify.AlbumTypeAlbum,
			spotify.AlbumTypeSingle,
			spotify.AlbumTypeCompilation,
			// spotify.AlbumTypeAppearsOn,
		}, spotify.Limit(50))
	if err != nil {
		return nil, requestCount, err
	}
	requestCount++

	allAlbums := []spotify.SimpleAlbum{}
	// cap to prevent infinite loop
	for range 100 {
		allAlbums = append(allAlbums, albums.Albums...)
		err = c.Client.NextPage(ctx, albums)
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
		fullAlbums, err := c.Client.GetAlbums(ctx, ids)
		if err != nil {
			return nil, requestCount, err
		}
		requestCount++
		for _, fullAlbum := range fullAlbums {
			tracks := fullAlbum.Tracks
			for range 100 {
				allSimpleTracks = append(allSimpleTracks, tracks.Tracks...)
				err = c.Client.NextPage(ctx, &tracks)
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

	artistIds := getAllArtists(&allSimpleTracks)
	allArtists := make(map[spotify.ID]*spotify.FullArtist)
	for chunk := range slices.Chunk(artistIds, 50) {
		r, err := c.Client.GetArtists(ctx, chunk...)
		if err != nil {
			return nil, requestCount, err
		}
		requestCount++
		for _, a := range r {
			allArtists[a.ID] = a
		}
	}

	allTracks := make([]*FullerTrack, len(allSimpleTracks))
	offset := 0
	for chunk := range slices.Chunk(allSimpleTracks, 100) {
		ids := make([]spotify.ID, len(chunk))
		for i, a := range chunk {
			ids[i] = a.ID
		}

		features, err := c.Client.GetAudioFeatures(ctx, ids...)
		if err != nil {
			return nil, requestCount, err
		}
		requestCount++

		fullTracks := []*spotify.FullTrack{}
		for subChunk := range slices.Chunk(ids, 50) {
			full, err := c.Client.GetTracks(ctx, subChunk, spotify.Limit(50))
			if err != nil {
				return nil, 0, err
			}
			requestCount++
			fullTracks = append(fullTracks, full...)
		}
		for i := range len(ids) {
			ft := &FullerTrack{
				Track:    fullTracks[i],
				Features: features[i],
			}
			for _, a := range ft.Track.Artists {
				ft.Artists = append(ft.Artists, allArtists[a.ID])
			}
			allTracks[i+offset] = ft
		}
		offset += 100
	}
	return allTracks, requestCount, nil
}
