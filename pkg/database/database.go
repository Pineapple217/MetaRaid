package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Pineapple217/MetaRaid/pkg/config"
	"github.com/Pineapple217/MetaRaid/pkg/helper"
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

func AddTask(rdb *redis.Client, ctx context.Context, task string) error {
	err := rdb.Watch(ctx, func(tx *redis.Tx) error {
		exists, err := tx.SIsMember(ctx, "processed_tasks", task).Result()
		if err != nil {
			return err
		}
		if exists {
			slog.Debug("Task has already been processed, not adding", "task", task)
			return nil
		}

		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.RPush(ctx, "task_queue", task)
			return nil
		})

		if err == nil {
			slog.Debug("Task added to the queue", "task", task)
		}
		return err
	}, "processed_tasks")

	return err
}

func AddTasks(rdb *redis.Client, ctx context.Context, tasks []spotify.ID) error {
	err := rdb.Watch(ctx, func(tx *redis.Tx) error {
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, t := range tasks {
				task := t.String()
				exists, err := tx.SIsMember(ctx, "processed_tasks", task).Result()
				if err != nil {
					return err
				}
				if !exists {
					pipe.RPush(ctx, "task_queue", task)
					slog.Debug("Task added to the queue", "task", task)
				} else {
					slog.Debug("Task has already been processed, not adding", "task", task)
				}
			}
			return nil
		})
		return err
	}, "processed_tasks")

	return err
}

func EnsureSeedTask(rdb *redis.Client, ctx context.Context, seedTask string) error {
	queueLength, err := rdb.LLen(ctx, "task_queue").Result()
	if err != nil {
		return err
	}

	if queueLength == 0 {
		slog.Info("Task queue is empty, adding seed task", "seed", seedTask)
		return AddTask(rdb, ctx, seedTask)
	}

	slog.Info("Task queue is not empty, no seed task needed")
	return nil
}

func GetTasks(rdb *redis.Client, ctx context.Context, count int) ([]string, error) {
	var tasks []string
	err := rdb.Watch(ctx, func(tx *redis.Tx) error {
		// Peek at the first 'limit' tasks in the queue
		taskList, err := tx.LRange(ctx, "task_queue", 0, int64(count-1)).Result()
		if err != nil {
			return err
		}
		if len(taskList) == 0 {
			slog.Warn("No more tasks to process.")
			time.Sleep(1000 * time.Millisecond)
			return nil
		}

		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, task := range taskList {
				pipe.LPop(ctx, "task_queue")
				pipe.SAdd(ctx, "in_progress_tasks", task)
			}
			return nil
		})

		tasks = taskList
		return err
	}, "task_queue")

	return tasks, err
}

func RecoverInProgressTasks(rdb *redis.Client, ctx context.Context) error {
	inProgressTasks, err := rdb.SMembers(ctx, "in_progress_tasks").Result()
	if err != nil {
		return err
	}

	for _, task := range inProgressTasks {
		slog.Info("Recovering task back to the queue", "task", task)
		if err := rdb.RPush(ctx, "task_queue", task).Err(); err != nil {
			return err
		}
		if err := rdb.SRem(ctx, "in_progress_tasks", task).Err(); err != nil {
			return err
		}
	}

	return nil
}
