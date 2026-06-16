-- +goose Up
-- Background job queue (internal/core/jobs). Optional: only used when
-- JOBS_ENABLED=true. Remove this migration + internal/core/jobs to drop it.
CREATE TABLE jobs (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind         text NOT NULL,
    payload      jsonb NOT NULL DEFAULT '{}',
    attempts     int NOT NULL DEFAULT 0,
    max_attempts int NOT NULL DEFAULT 25,
    run_at       timestamptz NOT NULL DEFAULT now(),
    locked_at    timestamptz,
    last_error   text,
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- Dequeue scans ready, unlocked jobs oldest-first; the partial index keeps that
-- hot path cheap.
CREATE INDEX jobs_ready_idx ON jobs (run_at) WHERE locked_at IS NULL;

-- +goose Down
DROP TABLE jobs;
