-- name: GetUserByIdentity :one
SELECT u.* FROM users u
JOIN user_identities i ON i.user_id = u.id
WHERE i.provider = $1 AND i.provider_user_id = $2;

-- name: CreateIdentity :exec
INSERT INTO user_identities (user_id, provider, provider_user_id)
VALUES ($1, $2, $3);

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSessionWithUser :one
SELECT s.expires_at AS session_expires_at, sqlc.embed(u)
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1;

-- name: GetWorkspaceOwnerLifecycle :one
SELECT u.deleted_at, u.purge_started_at
FROM workspaces w JOIN users u ON u.id = w.owner_user_id
WHERE w.id = sqlc.arg(workspace_id)::uuid;

-- name: LockWorkspaceObjectWrite :exec
SELECT pg_advisory_lock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: UnlockWorkspaceObjectWrite :one
SELECT pg_advisory_unlock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();

-- name: GetWorkspaceOwner :one
SELECT w.owner_user_id FROM workspaces w WHERE w.id = sqlc.arg(workspace_id)::uuid;

-- name: GetUserLifecycle :one
SELECT deleted_at, purge_started_at FROM users WHERE id = $1;
