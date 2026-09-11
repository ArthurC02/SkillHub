-- name: CreateUser :one
INSERT INTO users (email, display_name)
VALUES ($1, $2)
RETURNING *;

-- name: ListUserDisplayNames :many
SELECT id, display_name FROM users WHERE id = ANY(@user_ids::uuid[]);

-- name: FindLiveUserByEmail :one
SELECT u.id, u.email, u.display_name, u.created_at, u.deletion_requested_at, w.id AS workspace_id
FROM users u
JOIN workspaces w ON w.owner_user_id = u.id
WHERE lower(u.email) = lower(@email::text) AND u.deleted_at IS NULL;
