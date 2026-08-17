-- The dispatch halt switch (03:SEC-012, ADR-022 X-04). Four statements, and that
-- count is the point: both triggers declare through the same INSERT and both
-- releases go through the same UPDATE, which is what 03:SEC-012 means by 「必須共用
-- 同一份狀態與同一條解除路徑」.
--
-- None of these is workspace scoped, and that is not an iron rule 3 exception:
-- a halt is execution-plane state, takes no caller-supplied identifier beyond a
-- provider name from deployment configuration, and returns no user content.

-- name: ListActiveDispatchHalts :many
-- Everything currently stopping dispatch. Read on every run creation and every
-- dispatch, which is affordable because the table is empty in the normal case and
-- the partial unique index bounds it to one row per configured provider plus one
-- for the pool.
SELECT * FROM dispatch_halts
WHERE lifted_at IS NULL
ORDER BY declared_at;

-- name: DeclareDispatchHalt :one
-- Flip the switch for one target. Idempotent: a second declaration on a target
-- already halted updates the reason rather than failing, because a P1 arriving on
-- top of an X-04 pause is the same platform in the same state and 409 would only
-- invite a retry loop.
--
-- The CASE is the one asymmetry, and it is the escalation rule of 02:SEC-010 「不確定
-- 屬 P1 或 P2 時一律以 P1 處理」 written down: an incoming P1 takes over a halt the
-- reconciler raised, an incoming threshold breach never downgrades a P1. Without it
-- the reconciler's next round would quietly turn a security halt into a capacity one
-- and hand it an automatic release.
INSERT INTO dispatch_halts (provider, source, reason, declared_by)
VALUES (@provider, @source, @reason, sqlc.narg(declared_by))
ON CONFLICT (provider) WHERE lifted_at IS NULL DO UPDATE
SET source = CASE WHEN EXCLUDED.source = 'p1_incident' THEN EXCLUDED.source ELSE dispatch_halts.source END,
    reason = CASE WHEN EXCLUDED.source = 'p1_incident' OR dispatch_halts.source <> 'p1_incident'
                  THEN EXCLUDED.reason ELSE dispatch_halts.reason END,
    declared_by = CASE WHEN EXCLUDED.source = 'p1_incident' THEN EXCLUDED.declared_by ELSE dispatch_halts.declared_by END,
    -- Re-declaring resets the clock on any automatic recovery in progress.
    clear_rounds = 0
RETURNING *;

-- name: LiftDispatchHalt :one
-- The single release path. The `sources` predicate is how the reconciler is kept
-- from releasing a security halt: it passes {'orphan_threshold'}, the operator
-- endpoint passes both. Same statement, so there is no second way to resume
-- dispatch that could disagree with this one.
--
-- Returns no rows when nothing was halted, which callers treat as success: the
-- intent (dispatch must not be halted) is already satisfied.
UPDATE dispatch_halts
SET lifted_at = now(), lifted_by = sqlc.narg(lifted_by), lift_reason = @lift_reason
WHERE provider = @provider
  AND lifted_at IS NULL
  AND source = ANY (@sources::text[])
RETURNING *;

-- name: SetDispatchHaltClearRounds :one
-- ADR-022 X-04's hysteresis, kept on the row because the reconciler is a
-- leader-elected job whose leadership moves between processes. `clear` true counts
-- one more round below the threshold; false resets, because the requirement is two
-- CONSECUTIVE clear rounds.
--
-- Restricted to 'orphan_threshold': a P1 halt has no automatic recovery to make
-- progress on (03:SEC-012 「解除不得是自動的」), so it must not accumulate rounds that
-- some later change could act on.
UPDATE dispatch_halts
SET clear_rounds = CASE WHEN @clear::boolean THEN clear_rounds + 1 ELSE 0 END
WHERE provider = @provider AND lifted_at IS NULL AND source = 'orphan_threshold'
RETURNING clear_rounds;
