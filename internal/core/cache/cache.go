// Package cache is a minimal byte-oriented cache with two stdlib-only drivers: an
// in-process MemoryCache (TTL, lazy expiry) and a PostgresCache (a `cache` table).
// No runtime dependency beyond pgx.
//
// It is OPTIONAL and unwired: a feature builds a driver on demand —
// cache.NewMemory() or cache.NewPostgres(deps.Pool). The PostgresCache table ships
// as migration 00004 (project-owned scaffold). A ttl of 0 means "no expiry".
package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Cache stores opaque bytes under string keys. Implementations are safe for
// concurrent use. Get's second return is false on a miss (no error).
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// Remember returns the cached value for key, or computes it with fn, caches it
// for ttl, and returns it. It is the Cache equivalent of Laravel's Cache::remember.
func Remember(ctx context.Context, c Cache, key string, ttl time.Duration, fn func() ([]byte, error)) ([]byte, error) {
	if v, ok, err := c.Get(ctx, key); err != nil {
		return nil, err
	} else if ok {
		return v, nil
	}
	v, err := fn()
	if err != nil {
		return nil, err
	}
	if err := c.Set(ctx, key, v, ttl); err != nil {
		return nil, err
	}
	return v, nil
}

// ── MemoryCache ──────────────────────────────────────────────────────────────

type entry struct {
	value []byte
	exp   time.Time // zero = no expiry
}

// MemoryCache is a process-local cache. Entries expire lazily on Get.
type MemoryCache struct {
	mu    sync.RWMutex
	items map[string]entry
}

// NewMemory returns an empty in-process cache.
func NewMemory() *MemoryCache { return &MemoryCache{items: make(map[string]entry)} }

func (c *MemoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	if !e.exp.IsZero() && time.Now().After(e.exp) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil, false, nil
	}
	return e.value, true, nil
}

func (c *MemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	c.mu.Lock()
	c.items[key] = entry{value: append([]byte(nil), value...), exp: exp}
	c.mu.Unlock()
	return nil
}

func (c *MemoryCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
	return nil
}

// ── PostgresCache ────────────────────────────────────────────────────────────

// PostgresCache persists entries in the `cache` table (shared across replicas).
type PostgresCache struct{ pool *pgxpool.Pool }

// NewPostgres returns a Cache backed by the pool.
func NewPostgres(pool *pgxpool.Pool) *PostgresCache { return &PostgresCache{pool: pool} }

const (
	pgGetSQL = `SELECT value FROM cache WHERE key = $1 AND (expires_at IS NULL OR expires_at > now())`
	pgSetSQL = `INSERT INTO cache (key, value, expires_at) VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, expires_at = EXCLUDED.expires_at`
	pgDelSQL = `DELETE FROM cache WHERE key = $1`
)

func (c *PostgresCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	var value []byte
	err := c.pool.QueryRow(ctx, pgGetSQL, key).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (c *PostgresCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	var exp *time.Time
	if ttl > 0 {
		t := time.Now().Add(ttl)
		exp = &t
	}
	_, err := c.pool.Exec(ctx, pgSetSQL, key, value, exp)
	return err
}

func (c *PostgresCache) Delete(ctx context.Context, key string) error {
	_, err := c.pool.Exec(ctx, pgDelSQL, key)
	return err
}
