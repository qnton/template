package cache

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMemorySetGetDelete(t *testing.T) {
	ctx := context.Background()
	c := NewMemory()

	if _, ok, _ := c.Get(ctx, "missing"); ok {
		t.Error("Get on empty cache returned a hit")
	}
	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := c.Get(ctx, "k"); err != nil || !ok || string(v) != "v" {
		t.Fatalf("Get = (%q, %v, %v), want (v, true, nil)", v, ok, err)
	}
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.Get(ctx, "k"); ok {
		t.Error("Get after Delete returned a hit")
	}
}

func TestMemoryTTL(t *testing.T) {
	ctx := context.Background()
	c := NewMemory()
	if err := c.Set(ctx, "k", []byte("v"), 15*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.Get(ctx, "k"); !ok {
		t.Fatal("value should be present before TTL")
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok, _ := c.Get(ctx, "k"); ok {
		t.Error("value should have expired after TTL")
	}
}

func TestRemember(t *testing.T) {
	ctx := context.Background()
	c := NewMemory()
	calls := 0
	compute := func() ([]byte, error) { calls++; return []byte("computed"), nil }

	v, err := Remember(ctx, c, "k", time.Minute, compute)
	if err != nil || string(v) != "computed" {
		t.Fatalf("first Remember = (%q, %v)", v, err)
	}
	v, err = Remember(ctx, c, "k", time.Minute, compute)
	if err != nil || string(v) != "computed" {
		t.Fatalf("second Remember = (%q, %v)", v, err)
	}
	if calls != 1 {
		t.Errorf("compute called %d times, want 1 (second call should hit cache)", calls)
	}

	wantErr := errors.New("boom")
	if _, err := Remember(ctx, NewMemory(), "x", time.Minute, func() ([]byte, error) { return nil, wantErr }); !errors.Is(err, wantErr) {
		t.Errorf("Remember should surface fn error, got %v", err)
	}
}

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

func TestPostgresRoundTrip(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE cache"); err != nil {
		t.Skipf("cache table unavailable (run make migrate): %v", err)
	}
	c := NewPostgres(pool)

	if err := c.Set(ctx, "k", []byte("hello"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := c.Get(ctx, "k"); err != nil || !ok || string(v) != "hello" {
		t.Fatalf("Get = (%q, %v, %v), want (hello, true, nil)", v, ok, err)
	}
	// Upsert (ON CONFLICT) replaces the value.
	if err := c.Set(ctx, "k", []byte("world"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := c.Get(ctx, "k"); string(v) != "world" {
		t.Errorf("after upsert Get = %q, want world", v)
	}
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.Get(ctx, "k"); ok {
		t.Error("Get after Delete returned a hit")
	}
}
