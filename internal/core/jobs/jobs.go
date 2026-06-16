// Package jobs is a small, dependency-free background job queue backed by
// Postgres. Dequeue uses SELECT … FOR UPDATE SKIP LOCKED so multiple workers (or
// replicas) never process the same job. It adds no runtime dependency beyond pgx,
// which the template already uses.
//
// It is OPTIONAL: the worker only runs when JOBS_ENABLED=true. A feature enqueues
// work with jobs.New(deps.Pool).Enqueue(ctx, "<kind>", payload); handlers are
// registered in internal/feature/registry (RegisterJobs). The "jobs" table ships
// as a migration (project-owned scaffold) — delete it, this package, and the
// registry hook to remove the feature.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Client enqueues jobs.
type Client struct{ pool *pgxpool.Pool }

// New returns a Client bound to the pool.
func New(pool *pgxpool.Pool) *Client { return &Client{pool: pool} }

const enqueueSQL = `INSERT INTO jobs (kind, payload) VALUES ($1, $2)`

// Enqueue serialises payload to JSON and inserts a job of the given kind. A nil
// payload is stored as an empty JSON object.
func (c *Client) Enqueue(ctx context.Context, kind string, payload any) error {
	data := []byte("{}")
	if payload != nil {
		var err error
		if data, err = json.Marshal(payload); err != nil {
			return fmt.Errorf("jobs: marshal %q payload: %w", kind, err)
		}
	}
	if _, err := c.pool.Exec(ctx, enqueueSQL, kind, data); err != nil {
		return fmt.Errorf("jobs: enqueue %q: %w", kind, err)
	}
	return nil
}

// HandlerFunc processes a job's JSON payload. Returning an error reschedules the
// job with exponential backoff until its max_attempts is reached, after which it
// is parked (run_at = infinity) with last_error recorded for inspection.
type HandlerFunc func(ctx context.Context, payload []byte) error

// Options configures a Worker. Zero values fall back to sane defaults.
type Options struct {
	PollInterval time.Duration // default 1s
	JobTimeout   time.Duration // default 30s
}

// Worker polls the jobs table and runs registered handlers.
type Worker struct {
	pool     *pgxpool.Pool
	log      *slog.Logger
	handlers map[string]HandlerFunc
	poll     time.Duration
	timeout  time.Duration
}

// NewWorker builds a Worker. Register handlers with Handle before calling Run.
func NewWorker(pool *pgxpool.Pool, log *slog.Logger, opts Options) *Worker {
	if opts.PollInterval <= 0 {
		opts.PollInterval = time.Second
	}
	if opts.JobTimeout <= 0 {
		opts.JobTimeout = 30 * time.Second
	}
	return &Worker{
		pool:     pool,
		log:      log,
		handlers: map[string]HandlerFunc{},
		poll:     opts.PollInterval,
		timeout:  opts.JobTimeout,
	}
}

// Handle registers a handler for a job kind. Call during setup, before Run; it is
// not safe for concurrent registration.
func (w *Worker) Handle(kind string, fn HandlerFunc) { w.handlers[kind] = fn }

const dequeueSQL = `
UPDATE jobs SET locked_at = now(), attempts = attempts + 1
WHERE id = (
	SELECT id FROM jobs
	WHERE run_at <= now() AND locked_at IS NULL
	ORDER BY run_at
	FOR UPDATE SKIP LOCKED
	LIMIT 1
)
RETURNING id, kind, payload, attempts, max_attempts`

const (
	completeSQL = `DELETE FROM jobs WHERE id = $1`
	retrySQL    = `UPDATE jobs SET locked_at = NULL, run_at = now() + make_interval(secs => $2), last_error = $3 WHERE id = $1`
	deadSQL     = `UPDATE jobs SET run_at = 'infinity', last_error = $2 WHERE id = $1`
)

// Run polls until ctx is cancelled, draining all ready jobs each tick. It blocks;
// run it in a goroutine. On ctx cancellation it returns after the current job.
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(w.poll)
	defer t.Stop()
	w.log.Info("jobs worker started", slog.Duration("poll", w.poll))
	for {
		select {
		case <-ctx.Done():
			w.log.Info("jobs worker stopped")
			return
		case <-t.C:
			for {
				worked, err := w.processOne(ctx)
				if err != nil {
					w.log.ErrorContext(ctx, "jobs: process", slog.Any("error", err))
					break
				}
				if !worked {
					break // queue drained for this tick
				}
			}
		}
	}
}

// processOne claims and runs a single ready job. It reports whether a job was
// claimed (false = queue empty) so Run can drain in a loop.
func (w *Worker) processOne(ctx context.Context) (bool, error) {
	var (
		id          int64
		kind        string
		payload     []byte
		attempts    int
		maxAttempts int
	)
	err := w.pool.QueryRow(ctx, dequeueSQL).Scan(&id, &kind, &payload, &attempts, &maxAttempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	handler, ok := w.handlers[kind]
	if !ok {
		_, _ = w.pool.Exec(ctx, deadSQL, id, "no handler registered for kind "+kind)
		w.log.WarnContext(ctx, "jobs: no handler", slog.String("kind", kind), slog.Int64("id", id))
		return true, nil
	}

	jobCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	runErr := safeRun(jobCtx, handler, payload)
	switch {
	case runErr == nil:
		_, err = w.pool.Exec(ctx, completeSQL, id)
		return true, err
	case attempts >= maxAttempts:
		_, _ = w.pool.Exec(ctx, deadSQL, id, runErr.Error())
		w.log.ErrorContext(ctx, "jobs: job parked (max attempts)", slog.String("kind", kind), slog.Int64("id", id), slog.Any("error", runErr))
		return true, nil
	default:
		d := backoff(attempts)
		_, err = w.pool.Exec(ctx, retrySQL, id, int(d.Seconds()), runErr.Error())
		w.log.WarnContext(ctx, "jobs: retry scheduled", slog.String("kind", kind), slog.Int64("id", id), slog.Duration("in", d), slog.Any("error", runErr))
		return true, err
	}
}

// safeRun runs a handler, converting a panic into an error so one bad job cannot
// crash the worker.
func safeRun(ctx context.Context, fn HandlerFunc, payload []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn(ctx, payload)
}

// backoff is exponential (~2^attempt seconds) capped at one hour. The cap is
// applied before converting to a Duration so a large attempt can't overflow.
func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	secs := math.Pow(2, float64(attempt))
	if secs >= 3600 {
		return time.Hour
	}
	return time.Duration(secs) * time.Second
}
