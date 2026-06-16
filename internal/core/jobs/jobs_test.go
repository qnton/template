package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBackoff(t *testing.T) {
	cases := map[int]time.Duration{0: 2 * time.Second, 1: 2 * time.Second, 2: 4 * time.Second, 3: 8 * time.Second}
	for attempt, want := range cases {
		if got := backoff(attempt); got != want {
			t.Errorf("backoff(%d) = %v, want %v", attempt, got, want)
		}
	}
	if got := backoff(100); got != time.Hour {
		t.Errorf("backoff(100) = %v, want 1h cap", got)
	}
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// testPool connects to the test database, skipping the test when none is
// configured/reachable so `go test ./...` stays green without Postgres.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("no TEST_DATABASE_URL/DATABASE_URL set; skipping DB test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestQueueRoundTrip(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE jobs"); err != nil {
		t.Skipf("jobs table unavailable (run make migrate): %v", err)
	}

	if err := New(pool).Enqueue(ctx, "greet", map[string]string{"name": "ada"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	var got string
	w := NewWorker(pool, testLogger(), Options{})
	w.Handle("greet", func(_ context.Context, payload []byte) error {
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		got = p.Name
		return nil
	})

	worked, err := w.processOne(ctx)
	if err != nil || !worked {
		t.Fatalf("processOne = (%v, %v), want (true, nil)", worked, err)
	}
	if got != "ada" {
		t.Errorf("handler decoded name = %q, want ada", got)
	}

	var remaining int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Errorf("completed job not deleted: %d remaining", remaining)
	}
}

func TestRetryOnError(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE jobs"); err != nil {
		t.Skipf("jobs table unavailable: %v", err)
	}

	if err := New(pool).Enqueue(ctx, "boom", nil); err != nil {
		t.Fatal(err)
	}
	w := NewWorker(pool, testLogger(), Options{})
	w.Handle("boom", func(context.Context, []byte) error { return errors.New("nope") })

	if _, err := w.processOne(ctx); err != nil {
		t.Fatalf("processOne: %v", err)
	}

	var attempts int
	var locked *time.Time
	if err := pool.QueryRow(ctx, "SELECT attempts, locked_at FROM jobs LIMIT 1").Scan(&attempts, &locked); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
	if locked != nil {
		t.Error("locked_at should be NULL after a retry is rescheduled")
	}
}
