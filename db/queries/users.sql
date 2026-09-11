-- name: CreateUser :one
INSERT INTO users (email, display_name)
VALUES ($1, $2)
RETURNING *;

-- name: ListUserDisplayNames :many
SELECT id, display_name FROM users WHERE id = ANY(@user_ids::uuid[]);
