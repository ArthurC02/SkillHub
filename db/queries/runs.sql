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
--
-- failure_class rides along instead of getting its own UPDATE because the 0005 trigger
-- freezes a run the moment it goes terminal: a second statement would arrive one row
-- version too late and be rejected (RUN-006). coalesce keeps it write-once-ish — a
-- transition that has nothing to classify leaves whatever is already there.
UPDATE runs SET
    status = @to_status,
    status_reason = @reason,
    failure_class = coalesce(sqlc.narg(failure_class), failure_class),
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

-- name: SetRunProvider :one
-- RUN-005: the scheduler's decision, written before the first dispatch. runtime_snapshot
-- freezes what was matched against (provider, isolation level, resolved runtime version)
-- so a past run stays explainable after the provider's capability changes (ADR-003).
-- Terminal runs are excluded: the 0005 trigger would reject the write anyway, and losing
-- that race means the run ended while we were dispatching.
UPDATE runs SET provider = $3, runtime_snapshot = $4
WHERE id = $1 AND workspace_id = $2
  AND status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out')
RETURNING *;

-- name: ListActiveRuns :many
-- RUN-008: every run that is still supposed to be moving, oldest first. Read by the
-- supervisor at worker start and on its interval, to re-drive runs whose job was lost
-- with the process and to time out runs that outlived their wall clock.
--
-- Not workspace scoped, unlike everything above it, and that is not an iron-rule-3
-- exception: no user input reaches this statement and no user data leaves it. The
-- caller acts on runs it re-reads under their own workspace_id from the row itself.
SELECT * FROM runs
WHERE status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out')
ORDER BY created_at
LIMIT $1;

-- name: ListRunsNeedingCleanup :many
-- RUN-007: terminal runs whose sandbox has not been confirmed released. The one minute
-- floor keeps this from racing the cleanup job that the terminal transition just
-- enqueued; anything still here after that is a job that exhausted its retries.
SELECT * FROM runs
WHERE status IN ('succeeded', 'failed', 'cancelled', 'timed_out')
  AND cleanup_status <> 'cleaned'
  AND finished_at < now() - interval '1 minute'
ORDER BY finished_at
LIMIT $1;

-- name: SetRunCleanupStatus :one
-- RUN-002 "Run 結束後必須進入清理流程": cleanup outcome is recorded apart from the
-- execution outcome, and stays writable after the run froze (0005). Repeating a cleanup
-- is safe, so this has no expected-from guard — unlike TransitionRun, re-applying it is
-- the intended behaviour (iron rule 9).
UPDATE runs SET
    cleanup_status = @cleanup_status,
    cleanup_at = CASE
        WHEN @cleanup_status::run_cleanup_status IN ('cleaned', 'failed') THEN now()
        ELSE cleanup_at END
WHERE id = @run_id AND workspace_id = @workspace_id
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

-- name: GetRunAttemptForReconcile :one
-- RUN-007 orphan scanning. GET /runs?active=true on a provider echoes back the platform
-- identifiers it was dispatched with, and this turns one of them back into "is that run
-- still ours to keep alive?".
--
-- The only statement here that takes no workspace_id, because the caller has none: the
-- id arrives from a provider, not from a user, and the only action it can lead to is
-- DELETE on that same provider's own sandbox. Nothing about a run is returned that could
-- reach a user, and a bogus id simply finds no row.
SELECT a.id, a.run_id, a.workspace_id, a.provider, a.provider_run_id, a.finished_at,
       r.status, r.cleanup_status
FROM run_attempts a
JOIN runs r ON r.id = a.run_id
WHERE a.id = $1;

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
-- The publisher's scan (internal/outbox). Oldest first, so the backlog drains in the
-- order the domain changes committed.
SELECT * FROM outbox_events
WHERE published_at IS NULL
ORDER BY occurred_at, event_id
LIMIT $1;

-- name: MarkOutboxEventsPublished :execrows
-- The publisher hands events on *before* marking them, so a crash in between re-delivers
-- rather than drops (at-least-once, ADR-008). `published_at IS NULL` makes the marking
-- itself idempotent: a re-run of the same batch updates nothing and keeps the timestamp
-- of the delivery that actually happened first.
UPDATE outbox_events SET published_at = now()
WHERE event_id = ANY(@event_ids::uuid[]) AND published_at IS NULL;

-- name: ListOutboxEventsByAggregate :many
SELECT * FROM outbox_events
WHERE aggregate_type = $1 AND aggregate_id = $2
ORDER BY occurred_at, event_id;
