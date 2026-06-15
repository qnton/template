-- Queries for the example feature. sqlc generates type-safe, parameterized Go
-- from these into ./store. Never build SQL with string concatenation.

-- name: ListItems :many
SELECT id, title, created_at
FROM items
ORDER BY created_at DESC, id DESC
LIMIT $1;

-- name: CreateItem :one
INSERT INTO items (title)
VALUES ($1)
RETURNING id, title, created_at;

-- name: DeleteItem :exec
DELETE FROM items
WHERE id = $1;

-- name: CountItems :one
SELECT count(*) FROM items;
