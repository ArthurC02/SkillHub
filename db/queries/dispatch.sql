-- name: ListActiveDispatchHalts :many
SELECT * FROM dispatch_halts
WHERE lifted_at IS NULL
ORDER BY declared_at;

-- name: LockDispatchHaltTarget :exec
SELECT pg_advisory_xact_lock(hashtextextended('dispatch-halt:' || @provider::text, 0));

-- name: GetActiveDispatchHalt :one
SELECT * FROM dispatch_halts
WHERE provider = @provider AND lifted_at IS NULL
FOR UPDATE;

-- name: InsertDispatchHalt :one
INSERT INTO dispatch_halts (provider, source, reason, declared_by)
VALUES (@provider, @source, @reason, sqlc.narg(declared_by))
RETURNING *;

-- name: RedeclareDispatchHalt :one
UPDATE dispatch_halts
SET source = @source, reason = @reason, declared_by = sqlc.narg(declared_by), clear_rounds = @clear_rounds
WHERE id = @id AND lifted_at IS NULL
RETURNING *;

-- name: LiftDispatchHalt :one
UPDATE dispatch_halts
SET lifted_at = now(), lifted_by = sqlc.narg(lifted_by), lift_reason = @lift_reason
WHERE provider = @provider
  AND lifted_at IS NULL
  AND source = ANY (@sources::text[])
RETURNING *;

-- name: SetDispatchHaltClearRounds :one
UPDATE dispatch_halts
SET clear_rounds = clear_rounds + 1
WHERE provider = @provider AND lifted_at IS NULL AND source = ANY(@sources::text[])
RETURNING clear_rounds;
