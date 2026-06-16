-- +goose Up
-- Shared cache store for internal/core/cache (PostgresCache). Optional — only used
-- if a feature constructs cache.NewPostgres(deps.Pool). Remove with the package.
CREATE TABLE cache (
    key        text PRIMARY KEY,
    value      bytea NOT NULL,
    expires_at timestamptz
);

-- Supports cheap cleanup of expired rows (e.g. a scheduled task).
CREATE INDEX cache_expires_idx ON cache (expires_at);

-- +goose Down
DROP TABLE cache;
