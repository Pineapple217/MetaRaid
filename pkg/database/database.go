package database

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Pineapple217/MetaRaid/pkg/config"
	"github.com/Pineapple217/MetaRaid/pkg/helper"
	spt "github.com/Pineapple217/MetaRaid/pkg/spotify"
	"github.com/redis/go-redis/v9"
	"github.com/zmb3/spotify/v2"
)

func NewRedis(conf config.Redis) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", conf.Host, conf.Port),
		DB:           conf.Database,
		PoolSize:     conf.PoolSize,
		MinIdleConns: conf.MinIdleConns,
	})
	s := rdb.Ping(context.Background())
	helper.MaybeDie(s.Err(), "Failed to connect to redis DB")

	return rdb
}

// lua
func AddJobs(rdb *redis.Client, ctx context.Context, jobs []spotify.ID) error {
	pipe := rdb.TxPipeline()
	for _, job := range jobs {
		wasSet, err := pipe.HSetNX(ctx, "jobs:"+job.String(), "status", "pending").Result()
		// a := pipe.HSetNX(ctx, "jobs:"+job.String(), "status", "pending")
		// slog.Info("HSETNX repsonse", "str", a.String())
		if err != nil {
			return err
		}
		if !wasSet {
			slog.Info("job already exists", "job", job.String())
			continue
		}
		_, err = pipe.SAdd(ctx, "jobs_pending", job.String()).Result()
		if err != nil {
			return err
		}
		slog.Debug("job added to queue", "job", job.String())
	}
	_, err := pipe.Exec(ctx)
	return err
}

func EnsureSeedJob(rdb *redis.Client, ctx context.Context, seedTask string) error {
	queueLength, err := rdb.LLen(ctx, "jobs_pending").Result()
	if err != nil {
		return err
	}

	if queueLength == 0 {
		slog.Info("job queue is empty, adding seed task", "seed", seedTask)
		return AddJobs(rdb, ctx, []spotify.ID{spotify.ID(seedTask)})
	}

	slog.Info("job queue is not empty, no seed job needed")
	return nil
}

// denk ik lua
func PopJobs(rdb *redis.Client, ctx context.Context, count int) ([]string, error) {
	pipe := rdb.TxPipeline()

	jobs, err := pipe.SPopN(ctx, "jobs_pending", int64(count)).Result()
	if err != nil {
		return nil, err
	}

	if len(jobs) == 0 {
		return nil, nil
	}

	var jobsOut []string

	for _, job := range jobs {
		jobsOut = append(jobsOut, job)
		pipe.HSet(ctx, "jobs:"+job, "status", "working")
		pipe.SAdd(ctx, "jobs_working", job)
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}

	return jobsOut, nil
}

// denk ik lua
func RecoverInProgressTasks(rdb *redis.Client, ctx context.Context) error {
	pipe := rdb.TxPipeline()
	c, err := pipe.SCard(ctx, "jobs_working").Result()
	if err != nil {
		return err
	}
	if c == 0 {
		slog.Info("no jobs to recover")
		return nil
	}

	jobs, err := pipe.SPopN(ctx, "jobs_working", c).Result()
	if err != nil {
		return nil
	}

	for _, job := range jobs {
		pipe.SAdd(ctx, "jobs_pending", job)
		pipe.HSet(ctx, "job:"+job, "status", "pending")
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

// mss lua
func MarkJobDone(rdb *redis.Client, ctx context.Context, job string) error {
	pipe := rdb.TxPipeline()
	pipe.SRem(ctx, "jobs_working", job)
	pipe.SAdd(ctx, "jobs_done", job)
	pipe.HSet(ctx, "jobs:"+job, "status", "done")

	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}
	slog.Info("Marked task as done", "task", job)
	return nil
}

func InsertTracks(rdb *redis.Client, ctx context.Context, tracks []*spt.FullerTrack) error {
	pipe := rdb.Pipeline()

	for _, track := range tracks {
		key := "tracks:" + string(track.Track.ID)
		serializedData, err := track.Serialize()
		if err != nil {
			return fmt.Errorf("failed to serialize data: %w", err)
		}
		pipe.Set(ctx, key, serializedData, 0)
	}

	_, err := pipe.Exec(ctx)
	return err
}
