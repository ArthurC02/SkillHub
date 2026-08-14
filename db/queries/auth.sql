-- name: GetUserByIdentity :one
SELECT u.* FROM users u
JOIN user_identities i ON i.user_id = u.id
WHERE i.provider = $1 AND i.provider_user_id = $2 AND u.deleted_at IS NULL;

-- name: CreateIdentity :exec
INSERT INTO user_identities (user_id, provider, provider_user_id)
VALUES ($1, $2, $3);

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSessionUser :one
-- Session resolution and expiry check in one query; the caller derives user_id
-- and workspace from this row, never from the client (iron rule 3, ADR-011).
SELECT u.* FROM users u
JOIN sessions s ON s.user_id = u.id
WHERE s.token_hash = $1 AND s.expires_at > now() AND u.deleted_at IS NULL;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();
