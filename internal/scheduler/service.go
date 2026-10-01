package scheduler

import (
	"context"
	"log/slog"
	"strings"
	"time"

	gocron "github.com/go-co-op/gocron/v2"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

// Service wraps a gocron scheduler engine; register jobs separately before Start.
type Service struct {
	logger *slog.Logger
	cfg    config.Config
	sched  gocron.Scheduler
}

type schedulerFactory func(config.SchedulerConfig, *raftsvc.Service) (gocron.Scheduler, error)

func newSchedulerFactory() schedulerFactory {
	return newGocronScheduler
}

// New constructs the scheduler worker (not started until Start).
func New(cfg config.Config, logger *slog.Logger, raft *raftsvc.Service) (*Service, error) {
	return newWithSchedulerFactory(cfg, logger, raft, newSchedulerFactory())
}

func newWithSchedulerFactory(cfg config.Config, logger *slog.Logger, raft *raftsvc.Service, factory schedulerFactory) (*Service, error) {
	if factory == nil {
		factory = newGocronScheduler
	}
	s, err := factory(cfg.Scheduler, raft)
	if err != nil {
		return nil, err
	}
	return &Service{
		logger: logger,
		cfg:    cfg,
		sched:  s,
	}, nil
}

func newGocronScheduler(sc config.SchedulerConfig, raft *raftsvc.Service) (gocron.Scheduler, error) {
	opts := []gocron.SchedulerOption{
		gocron.WithLocation(time.Local),
	}

	if sc.RaftLeaderOnly {
		opts = append(opts, gocron.WithDistributedElector(newRaftElector(raft)))
	}

	if sc.MaxConcurrentJobs > 0 {
		mode := gocron.LimitModeReschedule
		if strings.EqualFold(strings.TrimSpace(sc.ConcurrentJobsMode), "wait") {
			mode = gocron.LimitModeWait
		}
		opts = append(opts, gocron.WithLimitConcurrentJobs(sc.MaxConcurrentJobs, mode))
	}

	s, err := gocron.NewScheduler(opts...)
	if err != nil {
		return nil, oopsx.B("scheduler").Wrapf(err, "new gocron scheduler")
	}
	return s, nil
}

// Jobs exposes the underlying gocron scheduler for registering tasks before Start.
func (s *Service) Jobs() gocron.Scheduler {
	return s.sched
}

// Start runs the scheduler (non-blocking). Register jobs before calling Start.
func (s *Service) Start(_ context.Context) error {
	s.sched.Start()
	s.logger.Info("scheduler started")
	return nil
}

// Stop shuts down the scheduler; it cannot be restarted afterward.
func (s *Service) Stop(ctx context.Context) error {
	if err := s.sched.ShutdownWithContext(ctx); err != nil {
		return oopsx.B("scheduler").Wrapf(err, "shutdown scheduler")
	}
	s.logger.Info("scheduler stopped")
	return nil
}
