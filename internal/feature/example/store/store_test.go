package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/app/internal/feature/example/store"
)

// These tests run against a REAL PostgreSQL instance (no DB mocking) when
// TEST_DATABASE_URL is set — point it at the docker-compose Postgres or a CI
// service container with migrations already applied. They are skipped otherwise,
// so `go test ./...` stays green without a database.
func newQueries(t *testing.T) (*store.Queries, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres store test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE items RESTART IDENTITY"); err != nil {
		pool.Close()
		t.Fatalf("truncate items (did you run `make migrate`?): %v", err)
	}
	return store.New(pool), pool
}

func TestItemsRoundTrip(t *testing.T) {
	q, pool := newQueries(t)
	defer pool.Close()
	ctx := context.Background()

	created, err := q.CreateItem(ctx, "first item")
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if created.ID == 0 || created.Title != "first item" {
		t.Fatalf("unexpected created item: %+v", created)
	}

	if _, err := q.CreateItem(ctx, "second item"); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	items, err := q.ListItems(ctx, 10)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ListItems len = %d, want 2", len(items))
	}
	// Ordered newest-first.
	if items[0].Title != "second item" {
		t.Errorf("ordering wrong: got %q first", items[0].Title)
	}

	n, err := q.CountItems(ctx)
	if err != nil || n != 2 {
		t.Fatalf("CountItems = %d, err = %v", n, err)
	}

	if err := q.DeleteItem(ctx, created.ID); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}
	if n, _ := q.CountItems(ctx); n != 1 {
		t.Fatalf("after delete count = %d, want 1", n)
	}
}
