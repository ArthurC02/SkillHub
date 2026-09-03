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
WHERE s.token_hash = $1 AND s.expires_at > now()
  AND u.deleted_at IS NULL AND u.purge_started_at IS NULL;

-- name: WorkspaceAcceptsObjects :one
-- Identity owns the account lifecycle gate. Object-producing contexts receive
-- this read through the composition root and evaluate it on their own locked
-- connection/transaction.
SELECT EXISTS (
    SELECT 1 FROM workspaces w JOIN users u ON u.id = w.owner_user_id
    WHERE w.id = sqlc.arg(workspace_id)::uuid
      AND u.deleted_at IS NULL AND u.purge_started_at IS NULL
);

-- name: LockWorkspaceObjectWrite :exec
SELECT pg_advisory_lock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: UnlockWorkspaceObjectWrite :one
SELECT pg_advisory_unlock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();
