-- name: ListActiveDispatchHalts :many
SELECT * FROM dispatch_halts
WHERE lifted_at IS NULL
ORDER BY declared_at;

-- name: DeclareDispatchHalt :one
INSERT INTO dispatch_halts (provider, source, reason, declared_by)
VALUES (@provider, @source, @reason, sqlc.narg(declared_by))
ON CONFLICT (provider) WHERE lifted_at IS NULL DO UPDATE
SET source = CASE WHEN EXCLUDED.source = 'p1_incident' THEN EXCLUDED.source ELSE dispatch_halts.source END,
    reason = CASE WHEN EXCLUDED.source = 'p1_incident' OR dispatch_halts.source <> 'p1_incident'
                  THEN EXCLUDED.reason ELSE dispatch_halts.reason END,
    declared_by = CASE WHEN EXCLUDED.source = 'p1_incident' THEN EXCLUDED.declared_by ELSE dispatch_halts.declared_by END,
    -- Re-declaring resets the clock on any automatic recovery already in progress.
    clear_rounds = 0
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
SET clear_rounds = CASE WHEN @clear::boolean THEN clear_rounds + 1 ELSE 0 END
WHERE provider = @provider AND lifted_at IS NULL AND source = 'orphan_threshold'
RETURNING clear_rounds;
