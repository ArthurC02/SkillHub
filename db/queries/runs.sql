-- Run orchestration (RUN-001~004, ADR-004). Every statement is workspace scoped
-- (iron rule 3), including the ones the worker runs: a job payload is not a wider
-- authority than a request, and the worker carries the workspace it was queued with.

-- Test case snapshots are created by testlab.CreateSnapshot, called inside the run
-- creation transaction: one implementation, one hash. See internal/testlab/snapshot.go.

-- name: CreateRun :one
INSERT INTO runs (
    workspace_id, skill_version_id, test_case_snapshot_id, provider,
    runtime_snapshot, policy_snapshot
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetRun :one
SELECT * FROM runs WHERE id = $1 AND workspace_id = $2;

-- name: TransitionRun :one
-- The state machine's only write. `status = @from_status` is the guard ADR-008 asks for:
-- a transition applied twice, or applied to a run something else already moved, updates
-- zero rows and the caller sees pgx.ErrNoRows rather than silently rewinding the run.
-- Legality of the pair itself is checked in Go before this runs (internal/run/state.go);
-- this only enforces that the run was still where the caller thought it was.
UPDATE runs SET
    status = @to_status,
    status_reason = @reason,
    started_at = CASE
        WHEN @to_status::run_status = 'running' AND started_at IS NULL THEN now()
        ELSE started_at END,
    finished_at = CASE
        WHEN @to_status::run_status IN ('succeeded', 'failed', 'cancelled', 'timed_out') THEN now()
        ELSE finished_at END
WHERE id = @run_id AND workspace_id = @workspace_id AND status = @from_status
RETURNING *;

-- name: RequestRunCancel :one
-- Records the user's intent only (RUN-004). The run stays in its current state until the
-- workload actually stops - propagating that to the provider is RUN-006. Idempotent:
-- asking twice keeps the first timestamp. Terminal runs match nothing, so a late cancel
-- is a 409 rather than a rewrite of a finished run.
UPDATE runs
SET cancel_requested_at = coalesce(cancel_requested_at, now())
WHERE id = $1 AND workspace_id = $2
  AND status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out')
RETURNING *;

-- name: InsertRunStatusTransition :exec
-- Append-only history; written in the same transaction as the status change it records
-- (RUN-002 "每次狀態變更皆記錄時間與原因").
INSERT INTO run_status_transitions (run_id, workspace_id, run_attempt_id, from_status, to_status, reason)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListRunStatusTransitions :many
SELECT t.* FROM run_status_transitions t
JOIN runs r ON r.id = t.run_id
WHERE t.run_id = $1 AND r.workspace_id = $2
ORDER BY t.occurred_at, t.id;

-- name: CreateRunAttempt :one
-- Allocates the next attempt number inline, the same way CreateSkillVersion allocates a
-- version number: a concurrent dispatch loses on run_attempts_run_number_key instead of
-- overwriting the attempt already there (RUN-003).
INSERT INTO run_attempts (run_id, workspace_id, attempt_number, provider)
SELECT r.id, r.workspace_id,
       (SELECT coalesce(max(attempt_number), 0) + 1 FROM run_attempts WHERE run_id = r.id),
       $3
FROM runs r
WHERE r.id = $1 AND r.workspace_id = $2
RETURNING *;

-- name: SetAttemptProviderRunID :one
-- The run_id -> provider_run_id mapping (RUN-003). It lands on the attempt, so a retry
-- adds a mapping and never erases the previous one.
UPDATE run_attempts SET provider_run_id = $3, started_at = coalesce(started_at, now())
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: FinishRunAttempt :one
-- error_class is RunError.class from contracts/openapi/sandbox-provider.yaml; NULL means
-- the attempt succeeded.
UPDATE run_attempts SET finished_at = now(), error_class = $3, error_message = $4
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: ListRunAttempts :many
SELECT * FROM run_attempts
WHERE run_id = $1 AND workspace_id = $2
ORDER BY attempt_number;

-- name: InsertOutboxEvent :one
-- Always written with the caller's transaction handle, so the event and the domain change
-- commit together or neither does (iron rule 9, ADR-008).
INSERT INTO outbox_events (
    event_type, event_version, correlation_id, causation_id,
    workspace_id, aggregate_type, aggregate_id, payload
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListUnpublishedOutboxEvents :many
-- The publisher's scan (RUN-005). No consumer exists yet; this is here so the backlog is
-- inspectable and so the test that proves a rolled-back transaction leaks no event has
-- something to read.
SELECT * FROM outbox_events
WHERE published_at IS NULL
ORDER BY occurred_at, event_id
LIMIT $1;

-- name: ListOutboxEventsByAggregate :many
SELECT * FROM outbox_events
WHERE aggregate_type = $1 AND aggregate_id = $2
ORDER BY occurred_at, event_id;
