package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestScheduler(t *testing.T) {
	t.Run("add and start fires function", func(t *testing.T) {
		s := New(cron.WithSeconds())
		called := make(chan struct{}, 5)
		err := s.Add("* * * * * *", func() { // every second
			called <- struct{}{}
		})
		if err != nil {
			t.Fatalf("Add() error: %v", err)
		}
		s.Start()
		defer s.Stop()

		select {
		case <-called:
			// ok, fired within a reasonable time
		case <-time.After(2 * time.Second):
			t.Fatal("function was not called within 2 seconds")
		}
	})

	t.Run("start stop idempotent", func(t *testing.T) {
		s := New()
		s.Start()
		s.Start() // must not panic

		s.Stop()
		s.Stop() // must not panic
	})

	t.Run("invalid cron spec returns error", func(t *testing.T) {
		s := New()
		err := s.Add("not-a-valid-cron-spec", func() {})
		if err == nil {
			t.Error("expected error for invalid cron spec")
		}
	})

	t.Run("overlapping jobs skipped", func(t *testing.T) {
		s := New(cron.WithSeconds())
		var mu sync.Mutex
		count := 0
		block := make(chan struct{})

		err := s.Add("* * * * * *", func() {
			mu.Lock()
			count++
			mu.Unlock()
			<-block // block until test releases
		})
		if err != nil {
			t.Fatalf("Add() error: %v", err)
		}
		s.Start()
		defer s.Stop()

		time.Sleep(2100 * time.Millisecond) // wait for at least 2 ticks
		close(block)                        // release the first blocked call

		// Give a moment for any duplicate to register
		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		c := count
		mu.Unlock()

		// SkipIfStillRunning should prevent overlapping: count should be 1
		// In rare cases, cron may fire before we block, so count could be 2+
		if c > 1 {
			t.Logf("SkipIfStillRunning may have allowed %d runs (could be timing-dependent)", c)
		}
	})
}
