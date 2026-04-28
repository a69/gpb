package scheduler

import (
	"log/slog"
	"sync"

	"github.com/robfig/cron/v3"
)

// Scheduler runs functions on cron schedules.
type Scheduler struct {
	cron *cron.Cron
	mu   sync.Mutex
}

// New creates a new scheduler.
func New() *Scheduler {
	return &Scheduler{
		cron: cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger))),
	}
}

// Add registers a function to run on the given cron spec.
func (s *Scheduler) Add(spec string, fn func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.cron.AddFunc(spec, func() {
		slog.Debug("cron job starting")
		fn()
		slog.Debug("cron job finished")
	})
	return err
}

// Start begins the scheduler.
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cron.Start()
	slog.Info("scheduler started")
}

// Stop waits for running jobs to finish, then stops the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cron.Stop()
	slog.Info("scheduler stopped")
}
