// Package schedule is a tiny in-process task scheduler built on stdlib time.
// Each task runs on its own ticker; a slow task naturally skips overlapping runs
// (Go tickers coalesce missed ticks), and a panicking task is recovered so it
// can't take down the process.
//
// It is OPTIONAL: the scheduler only runs when SCHEDULER_ENABLED=true. Register
// tasks in internal/feature/registry (RegisterSchedule). For multi-replica
// deployments, guard tasks that must run once (e.g. with a Postgres advisory
// lock) — an in-process scheduler fires on every replica.
package schedule

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// TaskFunc is a unit of scheduled work. Returning an error is logged; it does not
// stop the schedule.
type TaskFunc func(ctx context.Context) error

type task struct {
	name     string
	interval time.Duration
	fn       TaskFunc
}

// Scheduler holds registered tasks and runs them on intervals.
type Scheduler struct {
	log   *slog.Logger
	tasks []task
}

// New returns an empty Scheduler.
func New(log *slog.Logger) *Scheduler { return &Scheduler{log: log} }

// Every registers fn to run once per interval (first run after one interval).
// Call during setup, before Run.
func (s *Scheduler) Every(name string, interval time.Duration, fn TaskFunc) {
	s.tasks = append(s.tasks, task{name: name, interval: interval, fn: fn})
}

// Len reports how many tasks are registered.
func (s *Scheduler) Len() int { return len(s.tasks) }

// Run starts a ticker per task and blocks until ctx is cancelled, then waits for
// in-flight tasks to finish. Run it in a goroutine.
func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, t := range s.tasks {
		wg.Add(1)
		go func(t task) {
			defer wg.Done()
			s.loop(ctx, t)
		}(t)
	}
	s.log.Info("scheduler started", slog.Int("tasks", len(s.tasks)))
	<-ctx.Done()
	wg.Wait()
	s.log.Info("scheduler stopped")
}

func (s *Scheduler) loop(ctx context.Context, t task) {
	tk := time.NewTicker(t.interval)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			s.runTask(ctx, t)
		}
	}
}

func (s *Scheduler) runTask(ctx context.Context, t task) {
	defer func() {
		if r := recover(); r != nil {
			s.log.ErrorContext(ctx, "scheduled task panic", slog.String("task", t.name), slog.Any("recover", r))
		}
	}()
	if err := t.fn(ctx); err != nil {
		s.log.ErrorContext(ctx, "scheduled task failed", slog.String("task", t.name), slog.Any("error", err))
	}
}
