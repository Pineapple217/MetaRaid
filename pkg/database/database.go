package database

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

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

var addJobsScript = redis.NewScript(`
    local pendingKey = KEYS[1]
    local results = {}

    for i, jobKey in ipairs(ARGV) do
        local statusSet = redis.call("HSETNX", jobKey, "status", "pending")
        if statusSet == 1 then
            redis.call("SADD", pendingKey, jobKey)
        end
        results[i] = statusSet
    end

    return results
`)

func AddJobs(rdb *redis.Client, ctx context.Context, jobs []spotify.ID) error {
	jobKeys := make([]string, len(jobs))
	for i, job := range jobs {
		jobKeys[i] = "jobs:" + job.String()
	}

	results, err := addJobsScript.Run(ctx, rdb, []string{"jobs_pending"}, jobKeys).Result()
	if err != nil {
		return err
	}

	for i, job := range jobs {
		wasSet := results.([]any)[i].(int64)
		if wasSet == 1 {
			slog.Debug("job added to queue", "job", job.String())
		} else {
			slog.Debug("job already exists", "job", job.String())
		}
	}
	return nil
}

func EnsureSeedJob(rdb *redis.Client, ctx context.Context, seedTask string) error {
	queueLength, err := rdb.SCard(ctx, "jobs_pending").Result()
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

var popJobsScript = redis.NewScript(`
    local pendingKey = KEYS[1]
    local workingKey = KEYS[2]
    local count = tonumber(ARGV[1])
    local jobs = redis.call("SPOP", pendingKey, count)

    if #jobs == 0 then
        return {}
    end

    for i, job in ipairs(jobs) do
        local jobKey = "jobs:" .. job
        redis.call("HSET", jobKey, "status", "working")
        redis.call("SADD", workingKey, job)
    end

    return jobs
`)

func PopJobs(rdb *redis.Client, ctx context.Context, count int) ([]string, error) {
	results, err := popJobsScript.Run(ctx, rdb, []string{"jobs_pending", "jobs_working"}, count).Result()
	if err != nil {
		return nil, err
	}

	var jobsOut []string
	for _, job := range results.([]interface{}) {
		jobsOut = append(jobsOut, strings.TrimPrefix(job.(string), "jobs:"))
	}

	return jobsOut, nil
}

var recoverInProgressTasksScript = redis.NewScript(`
    local workingKey = KEYS[1]
    local pendingKey = KEYS[2]
    
    -- Get all jobs in jobs_working
    local jobs = redis.call("SMEMBERS", workingKey)
    
    if #jobs == 0 then
        return 0  -- No jobs to recover
    end

    -- Move each job to jobs_pending and update status
    for i, job in ipairs(jobs) do
        redis.call("SADD", pendingKey, job)
        redis.call("HSET", "jobs:" .. job, "status", "pending")
    end

    -- Remove all jobs from jobs_working
    redis.call("DEL", workingKey)

    return #jobs
`)

func RecoverInProgressTasks(rdb *redis.Client, ctx context.Context) error {
	result, err := recoverInProgressTasksScript.Run(ctx, rdb, []string{"jobs_working", "jobs_pending"}).Result()
	if err != nil {
		return err
	}

	if result.(int64) == 0 {
		slog.Info("no jobs to recover")
	} else {
		slog.Info("recovered jobs", "count", result.(int64))
	}

	return nil
}

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
