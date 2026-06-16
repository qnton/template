package schedule

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestEveryRunsTask(t *testing.T) {
	s := New(testLogger())
	ran := make(chan struct{}, 4)
	s.Every("ping", 5*time.Millisecond, func(context.Context) error {
		ran <- struct{}{}
		return nil
	})
	if s.Len() != 1 {
		t.Fatalf("Len = %d, want 1", s.Len())
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not run within 2s")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

// TestPanicRecovered confirms a panicking task does not stop the scheduler — a
// later run of the same task still fires.
func TestPanicRecovered(t *testing.T) {
	s := New(testLogger())
	var runs atomic.Int32
	s.Every("boom", 5*time.Millisecond, func(context.Context) error {
		runs.Add(1)
		panic("kaboom")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	s.Run(ctx)

	if runs.Load() < 2 {
		t.Errorf("task ran %d times; expected the scheduler to keep firing after a panic", runs.Load())
	}
}
