-- name: CreateWorkspace :one
INSERT INTO workspaces (owner_user_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: GetWorkspace :one
SELECT * FROM workspaces
WHERE id = $1 AND owner_user_id = $2;

-- name: ListWorkspacesByOwner :many
SELECT * FROM workspaces
WHERE owner_user_id = $1
ORDER BY created_at;
