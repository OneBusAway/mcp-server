-- name: GetEntry :one
SELECT data, expires_at FROM cache_entries WHERE key = ?;

-- name: SetEntry :exec
INSERT OR REPLACE INTO cache_entries (key, data, expires_at)
VALUES (?, ?, ?);

-- name: PruneExpired :exec
DELETE FROM cache_entries WHERE expires_at < ?;
