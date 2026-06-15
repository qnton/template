// Package db owns the PostgreSQL connection pool. The pool is the single
// swappable data-access seam: it is built once at startup from config and
// injected into features via Deps — never created per request.
//
// Features run queries through their sqlc-generated Queries, whose DBTX
// interface is satisfied by *pgxpool.Pool (and by pgx.Tx for transactions), so
// switching driver/database or pointing at a test DB touches only this package.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/app/internal/core/config"
)

// New builds a pgx connection pool from config and verifies connectivity with a
// Ping (bounded by ctx). pgx is pure Go, so CGO_ENABLED=0 and the static binary
// are preserved. The caller owns the returned pool and must Close it on shutdown.
func New(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	pcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	pcfg.MaxConns = cfg.DBMaxConns
	pcfg.MinConns = cfg.DBMinConns
	pcfg.MaxConnLifetime = cfg.DBMaxConnLifetime
	pcfg.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	pcfg.HealthCheckPeriod = cfg.DBHealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
