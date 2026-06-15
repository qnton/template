-- +goose Up
-- The example feature's table. goose applies these at deploy time; sqlc also
-- reads this directory as its schema source (it understands goose annotations).
CREATE TABLE items (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title      TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Supports the ListItems ORDER BY created_at DESC, id DESC.
CREATE INDEX items_created_at_id_idx ON items (created_at DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS items;
