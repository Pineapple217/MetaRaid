package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Pineapple217/MetaRaid/pkg/config"
	"github.com/Pineapple217/MetaRaid/pkg/database"
	"github.com/Pineapple217/MetaRaid/pkg/helper"
	spt "github.com/Pineapple217/MetaRaid/pkg/spotify"
	"github.com/redis/go-redis/v9"
	"github.com/zmb3/spotify/v2"
)

type Scraper struct {
	Clients []*spotify.Client
	RDB     *redis.Client
	Config  config.Scraper
	Wg      sync.WaitGroup
	Jobs    chan string
	workers []*Worker
	ctx     context.Context
	cancel  context.CancelFunc
}

type Worker struct {
	client       *spotify.Client
	id           string
	logger       *slog.Logger
	rdb          *redis.Client
	ctx          context.Context
	cancel       context.CancelFunc
	requestCount int64
	trackCount   int64
	status       status
}

type status int

const (
	initialized status = iota
	running
	coldKey
	stopped
)

func NewScraper(clients []*spotify.Client, rdb *redis.Client, conf config.Scraper) *Scraper {
	ctx, cancel := context.WithCancel(context.Background())
	ws := []*Worker{}

	for i, c := range clients {
		name := fmt.Sprintf("%d", i)
		logger := slog.With(slog.Group("worker"), slog.String("id", name))
		workerCtx, workerCancel := context.WithCancel(ctx)
		ws = append(ws, &Worker{
			client: c,
			id:     name,
			logger: logger,
			rdb:    rdb,
			ctx:    workerCtx,
			cancel: workerCancel,
			status: initialized,
		})
	}

	s := Scraper{
		Clients: clients,
		RDB:     rdb,
		Config:  conf,
		Wg:      sync.WaitGroup{},
		Jobs:    make(chan string, 20),
		workers: ws,
		ctx:     ctx,
		cancel:  cancel,
	}
	return &s
}

func (s *Scraper) Start() {
	slog.Info("Starting scraper")
	ctx := context.Background()
	err := database.RecoverInProgressTasks(s.RDB, ctx)
	helper.MaybeDieErr(err)
	err = database.EnsureSeedJob(s.RDB, ctx, s.Config.SeedArtistId)
	helper.MaybeDieErr(err)
	go s.fetchJobs()
	go s.runWorkers()
	go s.workerManage()
}

func (s *Scraper) Stop() {
	slog.Info("Stopping scraper")
	s.cancel()
	s.Wg.Wait()
}

func (w *Worker) Start(wg *sync.WaitGroup, jobs chan string) {
	if w.status != initialized {
		slog.Warn("Can not start worker, incorrect status", "status", w.status)
		return
	}
	w.logger.Info("Starting")
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-w.ctx.Done():
				return
			case <-ticker.C:
				r := atomic.SwapInt64(&w.requestCount, 0)
				t := atomic.SwapInt64(&w.trackCount, 0)
				w.logger.Info("Stats per minute", "request", r, "tracks", t)
			}
		}
	}()
	go func() {
		defer wg.Done()
		ctx := context.Background()
		for {
			select {
			case <-w.ctx.Done():
				w.logger.Info("stopped worker")
				return
			default:
				if len(jobs) == 0 {
					time.Sleep(time.Second)
					continue
				}
				job := <-jobs
				w.logger.Info("working", "job", job)
				fs, c, err := spt.FetchArtistTracks(w.client, ctx, spotify.ID(job))
				if err == spotify.ErrMaxRetryDurationExceeded {
					w.logger.Warn("Max retry duration exceeded, cold key")
					w.status = coldKey
					w.Stop()
					return
				}
				helper.MaybeDie(err, "Failed to ferch artists tracks")
				w.logger.Info("tracks fetched", "artist", job, "count", len(fs), "request_count", c)

				err = database.InsertTracks(w.rdb, ctx, fs)
				helper.MaybeDie(err, "Failed to add tracks")

				as := spt.GetArtists(fs, spotify.ID(job))
				err = database.AddJobs(w.rdb, ctx, as)
				helper.MaybeDie(err, "Failed to add tasks")

				atomic.AddInt64(&w.requestCount, int64(c))
				atomic.AddInt64(&w.trackCount, int64(len(fs)))

				err = database.MarkJobDone(w.rdb, ctx, job)
				if err != nil {
					w.logger.Error("Failed to mark job as done", "job", job)
				}
			}
		}
	}()
	w.status = running
}

func (w *Worker) Stop() {
	w.logger.Info("stopping")
	w.cancel()
	w.status = stopped
}

func (s *Scraper) runWorkers() {
	s.Wg.Add(len(s.workers))
	for _, w := range s.workers {
		w.Start(&s.Wg, s.Jobs)
	}
	s.Wg.Wait()
}

func (s *Scraper) workerManage() {
	s.Wg.Add(1)
	defer s.Wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			slog.Info("stopped workerManager")
			return
		case <-ticker.C:
			stp := 0
			r := 0
			for _, w := range s.workers {
				switch w.status {
				case stopped:
					stp++
				case running:
					r++
				}
			}
			slog.Info("worker pool state", "stopped", stp, "running", r)
			if r == 0 {
				go syscall.Kill(os.Getpid(), syscall.SIGINT)
			}
		}
	}
}

func (s *Scraper) fetchJobs() {
	s.Wg.Add(1)
	defer s.Wg.Done()
	ctx := context.Background()
	for {
		select {
		case <-s.ctx.Done():
			slog.Info("stopped job fetcher")
			return
		default:
			if len(s.Jobs) > 10 {
				time.Sleep(time.Second)
				continue
			}
			tasks, err := database.PopJobs(s.RDB, ctx, 5)
			if err != nil {
				slog.Warn("failed to fetch tasks", "error", err)
			}
			if len(tasks) == 0 {
				slog.Info("no jobs to fetch, waiting", "sec", 3)
				time.Sleep(time.Second * 3)
			} else {
				slog.Info("adding tracks to task queue", "count", len(tasks))
			}
			for _, task := range tasks {
				s.Jobs <- task
			}
		}
	}
}
