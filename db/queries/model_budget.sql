-- name: ListModelCallBudgets :many
SELECT * FROM model_call_budgets
ORDER BY kind;

-- name: SetModelCallBudget :one
INSERT INTO model_call_budgets (kind, seconds, reason, set_by)
VALUES (@kind, @seconds, @reason, sqlc.narg(set_by))
ON CONFLICT (kind) DO UPDATE
SET seconds = EXCLUDED.seconds, reason = EXCLUDED.reason, set_by = EXCLUDED.set_by, set_at = now()
RETURNING *;

-- name: ClearModelCallBudget :one
DELETE FROM model_call_budgets
WHERE kind = @kind
RETURNING *;
