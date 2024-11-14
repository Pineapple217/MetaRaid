package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"log/slog"
	"time"

	"github.com/Pineapple217/MetaRaid/pkg/config"
	"github.com/Pineapple217/MetaRaid/pkg/database"
	"github.com/Pineapple217/MetaRaid/pkg/helper"
	"github.com/Pineapple217/MetaRaid/pkg/spotify"
	"github.com/marcboeker/go-duckdb"
	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
	s "github.com/zmb3/spotify/v2"
)

func main() {
	ctx := context.Background()

	conf, err := config.Load()
	helper.MaybeDie(err, "Failed to load configs")

	rdb := database.NewRedis(conf.Redis)

	defer rdb.Close()

	connector, err := duckdb.NewConnector("meta_raid.duckdb", nil)
	if err != nil {
		slog.Error("failed to create DuckDB connector", slog.Any("error", err))
		return
	}

	conn, err := connector.Connect(ctx)
	if err != nil {
		slog.Error("failed to connect to DuckDB", slog.Any("error", err))
		return
	}
	defer conn.Close()

	db := sql.OpenDB(connector)
	defer db.Close()
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tracks (
			track_id STRING,
			name STRING,
			artis STRING,
			album STRING,
			album_type STRING,
			release_date DATE,
			image_l STRING,
			image_m STRING,
			image_s STRING,

			popularity INTEGER,
			acousticness FLOAT,
			danceability FLOAT,
			energy FLOAT,
			instrumentalness FLOAT,
			liveness FLOAT,
			speechiness FLOAT,
			valence FLOAT,
			key INTEGER,
			mode INTEGER,
			tempo FLOAT,
			time_signature INTEGER,
			loudness FLOAT,
			duration INTEGER,

			explicit BOOLEAN,
			preview_url STRING,
			type STRING,
			 -- playable BOOLEAN,
		)
	`)
	if err != nil {
		slog.Error("failed to create table 'tracks'", slog.Any("error", err))
		return
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tracks_parts (
			track_id STRING,
			name STRING,
			artis STRING,
		)
	`)
	if err != nil {
		slog.Error("failed to create table 'tracks_parts'", slog.Any("error", err))
		return
	}

	startTime := time.Now()
	Export(conn, rdb, ctx)
	duration := time.Since(startTime)

	var c uint64
	r := db.QueryRow(`select count(*) from tracks`)
	helper.MaybeDie(err, "query fail")
	r.Scan(&c)
	slog.Info("Export done", "row_count", c, "duration", duration, "per_sec", float64(c)/duration.Seconds())
}

func Export(conn driver.Conn, rdb *redis.Client, ctx context.Context) error {
	batchSize := int64(1000)
	var cursor uint64
	prefix := "tracks:"
	appender, err := duckdb.NewAppenderFromConn(conn, "", "tracks")
	helper.MaybeDieErr(err)
	defer appender.Close()
	appenderPart, err := duckdb.NewAppenderFromConn(conn, "", "tracks_parts")
	helper.MaybeDieErr(err)
	defer appenderPart.Close()
	for {
		keys, newCursor, err := rdb.Scan(ctx, cursor, prefix+"*", batchSize).Result()
		helper.MaybeDieErr(err)
		cursor = newCursor

		for _, key := range keys {
			a := rdb.Get(ctx, key)
			helper.MaybeDieErr(a.Err())
			b, err := a.Bytes()
			helper.MaybeDieErr(err)
			var record spotify.FullerTrack
			err = msgpack.Unmarshal(b, &record)
			helper.MaybeDieErr(err)
			if record.Features == nil {
				err = appenderPart.AppendRow(
					record.Track.ID.String(),
					record.Track.Name,
					record.Track.Artists[0].Name,
				)
				helper.MaybeDieErr(err)
				continue
			}
			err = appender.AppendRow(
				record.Track.ID.String(),
				record.Track.Name,
				record.Track.Artists[0].Name,
				record.Track.Album.Name,
				record.Track.Album.AlbumType,
				record.Track.Album.ReleaseDateTime(),
				GetImage(record.Track.Album.Images, 0),
				GetImage(record.Track.Album.Images, 1),
				GetImage(record.Track.Album.Images, 2),

				int32(record.Track.Popularity),
				record.Features.Acousticness,
				record.Features.Danceability,
				record.Features.Energy,
				record.Features.Instrumentalness,
				record.Features.Liveness,
				record.Features.Speechiness,
				record.Features.Valence,
				int32(record.Features.Key),
				int32(record.Features.Mode),
				record.Features.Tempo,
				int32(record.Features.TimeSignature),
				record.Features.Loudness,
				int32(record.Features.Duration),

				record.Track.Explicit,
				record.Track.PreviewURL,
				record.Track.Type,
				// record.Track.IsPlayable,
			)
			helper.MaybeDieErr(err)
		}
		slog.Info("fetches tracks", "count", len(keys), "offset", cursor)

		if newCursor == 0 {
			break
		}
	}
	return nil
}

func GetImage(imgs []s.Image, i int) string {
	if len(imgs) > i {
		return imgs[i].URL
	}
	return ""
}
