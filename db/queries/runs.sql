-- name: CreateRun :one
INSERT INTO runs (
    workspace_id, skill_version_id, test_case_snapshot_id, provider,
    runtime_snapshot, policy_snapshot
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetRun :one
SELECT * FROM runs WHERE id = $1 AND workspace_id = $2;

-- name: ListWorkspaceRuns :many
SELECT r.id, r.status, r.status_reason, r.provider, r.failure_class,
       r.cleanup_status, r.skill_version_id, r.test_case_snapshot_id,
       r.cancel_requested_at, r.created_at, r.started_at, r.finished_at,
       v.skill_id, sk.name AS skill_name, s.test_case_id
FROM runs r
JOIN skill_versions v ON v.id = r.skill_version_id
JOIN skills sk ON sk.id = v.skill_id
JOIN test_case_snapshots s ON s.id = r.test_case_snapshot_id
WHERE r.workspace_id = @workspace_id
  AND (sqlc.narg(test_case_id)::uuid IS NULL OR s.test_case_id = sqlc.narg(test_case_id)::uuid)
ORDER BY r.created_at DESC, r.id
LIMIT @page_size OFFSET @page_offset;

-- name: SoftDeleteRunArtifact :one
UPDATE artifacts SET deleted_at = now()
WHERE id = @artifact_id AND run_id = @run_id AND workspace_id = @workspace_id
  AND kind = 'run_output' AND deleted_at IS NULL
RETURNING id, object_key, purged_at;

-- name: GetRunArtifactForDelete :one
SELECT id, object_key, purged_at FROM artifacts
WHERE id = @artifact_id AND run_id = @run_id AND workspace_id = @workspace_id
  AND kind = 'run_output' AND deleted_at IS NULL;

-- name: LockRunArtifactObjectSession :exec
SELECT pg_advisory_lock(hashtextextended(@lock_key::text, 0));

-- name: UnlockRunArtifactObjectSession :one
SELECT pg_advisory_unlock(hashtextextended(@lock_key::text, 0));

-- name: GetRunLinkage :one
SELECT v.skill_id, s.test_case_id
FROM runs r
JOIN skill_versions v ON v.id = r.skill_version_id
JOIN test_case_snapshots s ON s.id = r.test_case_snapshot_id
WHERE r.id = @run_id AND r.workspace_id = @workspace_id;

-- name: TransitionRun :one
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
UPDATE runs
SET cancel_requested_at = coalesce(cancel_requested_at, now())
WHERE id = $1 AND workspace_id = $2
  AND status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out')
RETURNING *;

-- name: SetRunProvider :one
UPDATE runs SET provider = $3, runtime_snapshot = $4
WHERE id = $1 AND workspace_id = $2
  AND status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out')
RETURNING *;

-- name: ListActiveRuns :many
WITH candidates AS (
    SELECT id FROM runs
    WHERE status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out')
	  -- 30 seconds is one supervisor interval.
	  AND (supervision_checked_at IS NULL OR supervision_checked_at < now() - interval '30 seconds')
    ORDER BY supervision_checked_at NULLS FIRST, supervision_checked_at, created_at, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE runs r SET supervision_checked_at = now()
FROM candidates c WHERE r.id = c.id
RETURNING r.*;

-- name: ListRunsNeedingCleanup :many
WITH candidates AS (
    SELECT id FROM runs
    WHERE status IN ('succeeded', 'failed', 'cancelled', 'timed_out')
      AND cleanup_status <> 'cleaned'
      AND finished_at < now() - interval '1 minute'
	  AND (cleanup_attempted_at IS NULL OR cleanup_attempted_at < now() - interval '30 seconds')
    ORDER BY cleanup_attempted_at NULLS FIRST, cleanup_attempted_at, finished_at, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE runs r SET cleanup_attempted_at = now()
FROM candidates c WHERE r.id = c.id
RETURNING r.*;

-- name: SetRunCleanupStatus :one
UPDATE runs SET
    cleanup_status = @cleanup_status,
    cleanup_at = CASE
        WHEN @cleanup_status::run_cleanup_status IN ('cleaned', 'failed') THEN now()
        ELSE cleanup_at END
WHERE id = @run_id AND workspace_id = @workspace_id
RETURNING *;

-- name: InsertRunStatusTransition :exec
INSERT INTO run_status_transitions (run_id, workspace_id, run_attempt_id, from_status, to_status, reason)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListRunStatusTransitions :many
SELECT t.* FROM run_status_transitions t
JOIN runs r ON r.id = t.run_id
WHERE t.run_id = $1 AND r.workspace_id = $2
ORDER BY t.occurred_at, t.id;

-- name: CreateRunAttempt :one
INSERT INTO run_attempts (run_id, workspace_id, attempt_number, provider, object_grants_state)
SELECT r.id, r.workspace_id,
       (SELECT coalesce(max(attempt_number), 0) + 1 FROM run_attempts WHERE run_id = r.id),
       $3, 'unissued'
FROM runs r
WHERE r.id = $1 AND r.workspace_id = $2
RETURNING *;

-- name: SetAttemptProviderRunID :one
UPDATE run_attempts SET provider_run_id = $3, started_at = coalesce(started_at, now())
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: FinishRunAttempt :one
UPDATE run_attempts
SET finished_at = now(), error_class = $3, error_message = $4,
    object_grants_expire_at = CASE
        WHEN object_grants_state = 'unissued'
            THEN now() - interval '2 minutes'
        ELSE object_grants_expire_at
    END,
    object_grants_state = CASE
        WHEN object_grants_state = 'unissued' THEN 'closed'
        ELSE object_grants_state
    END
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: GetRunAttemptForReconcile :one
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
INSERT INTO outbox_events (
    event_type, event_version, correlation_id, causation_id,
    workspace_id, aggregate_type, aggregate_id, payload
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListUnpublishedOutboxEvents :many
SELECT * FROM outbox_events
WHERE published_at IS NULL AND dead_lettered_at IS NULL
ORDER BY occurred_at, event_id
LIMIT $1;

-- name: RecordOutboxDeliveryFailure :one
UPDATE outbox_events
SET delivery_attempts = delivery_attempts + 1,
    dead_lettered_at = CASE
        WHEN dead_lettered_at IS NOT NULL THEN dead_lettered_at
        WHEN delivery_attempts + 1 >= @max_attempts::int THEN now()
    END
WHERE event_id = @event_id
RETURNING delivery_attempts, dead_lettered_at;

-- name: DeleteOutboxEventsPublishedBefore :execrows
DELETE FROM outbox_events
WHERE published_at IS NOT NULL
  AND published_at < @published_before::timestamptz
  AND dead_lettered_at IS NULL;

-- name: MarkOutboxEventsPublished :execrows
UPDATE outbox_events SET published_at = now()
WHERE event_id = ANY(@event_ids::uuid[]) AND published_at IS NULL;

-- name: CountDeadLetteredOutboxEvents :one
SELECT count(*)::bigint FROM outbox_events WHERE dead_lettered_at IS NOT NULL;

-- name: ListOutboxEventsByAggregate :many
SELECT * FROM outbox_events
WHERE aggregate_type = $1 AND aggregate_id = $2
ORDER BY occurred_at, event_id;

-- name: LockWorkspaceRunSlots :exec
SELECT pg_advisory_xact_lock(hashtextextended(@workspace_id::text, 0));

-- name: CountActiveRuns :one
SELECT count(*) FROM runs
WHERE workspace_id = @workspace_id
  AND status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out');

-- name: InsertRunArtifact :execrows
INSERT INTO artifacts (
    workspace_id, run_id, kind, file_name, content_type, size_bytes, content_hash,
    object_key, expires_at
)
SELECT @workspace_id, @run_id, 'run_output', @file_name, @content_type, @size_bytes,
       @content_hash, @object_key, now() + interval '90 days'
WHERE NOT EXISTS (
    SELECT 1 FROM artifacts
    WHERE run_id = @run_id AND kind = 'run_output'
      AND lower(file_name) = lower(sqlc.arg(file_name)::text)
);

-- name: LockRunArtifactManifest :exec
SELECT pg_advisory_xact_lock(hashtextextended(
    'run-artifact-manifest:' || sqlc.arg(run_id)::uuid::text, 0));

-- name: MarkRunArtifactsTruncated :execrows
UPDATE runs
SET artifacts_truncated = true
WHERE id = @id AND workspace_id = @workspace_id;

-- name: ListRunArtifacts :many
SELECT * FROM artifacts
WHERE run_id = $1 AND workspace_id = $2 AND kind = 'run_output' AND deleted_at IS NULL
ORDER BY file_name;

-- name: ListReadableRunArtifacts :many
SELECT * FROM artifacts
WHERE run_id = $1 AND workspace_id = $2 AND kind = 'run_output'
  AND deleted_at IS NULL AND purged_at IS NULL AND expires_at > now()
ORDER BY file_name;

-- name: CountUnreadableRunArtifacts :one
SELECT
  count(*) FILTER (WHERE deleted_at IS NOT NULL)::bigint AS deleted,
  count(*) FILTER (WHERE deleted_at IS NULL
                     AND (purged_at IS NOT NULL OR expires_at <= now()))::bigint AS expired
FROM artifacts
WHERE run_id = $1 AND workspace_id = $2 AND kind = 'run_output';

-- name: RecordOrphanSighting :one
INSERT INTO reconciler_orphan_sightings (provider, provider_run_id)
VALUES (@provider, @provider_run_id)
ON CONFLICT (provider, provider_run_id) DO UPDATE
SET rounds = reconciler_orphan_sightings.rounds + 1, last_seen_at = now()
RETURNING rounds;

-- name: ForgetClearedOrphans :exec
DELETE FROM reconciler_orphan_sightings
WHERE provider = @provider
  AND NOT (provider_run_id = ANY(@still_present::text[]));

-- name: CountPersistentOrphans :one
SELECT count(*) FROM reconciler_orphan_sightings
WHERE provider = @provider AND rounds >= 2;
-- name: AccountPurgeReady :one
SELECT NOT EXISTS (
    SELECT 1 FROM runs r
    WHERE r.workspace_id = $1
      AND (r.status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out')
           OR r.cleanup_status <> 'cleaned'
           OR EXISTS (
               SELECT 1 FROM run_attempts a
               WHERE a.run_id = r.id
                 -- Waits one extra minute because S3 and Postgres clocks may differ.
                 AND (a.object_grants_state = 'legacy_unknown'
                      OR a.object_grants_expire_at > now() - interval '1 minute')
           ))
);

-- name: SetRunAttemptObjectGrantsExpiry :execrows
UPDATE run_attempts
SET object_grants_expire_at = @expires_at::timestamptz,
    object_grants_state = 'recorded'
WHERE id = @id AND workspace_id = @workspace_id;

-- name: CloseUnissuedRunAttemptGrants :execrows
UPDATE run_attempts
SET object_grants_expire_at = now() - interval '2 minutes',
    object_grants_state = 'closed'
WHERE run_id = @run_id AND workspace_id = @workspace_id
  AND object_grants_state = 'unissued';

-- name: ListRunArtifactUploadIntents :many
WITH candidates AS (
    SELECT id FROM run_artifact_upload_intents
    WHERE not_before <= now()
      AND (attempted_at IS NULL OR attempted_at < now() - interval '15 minutes')
    ORDER BY attempted_at NULLS FIRST, attempted_at, not_before, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE run_artifact_upload_intents i SET attempted_at = now()
FROM candidates c WHERE i.id = c.id
RETURNING i.id, i.workspace_id, i.object_key;

-- name: MarkRunArtifactUploadIntentPurged :exec
DELETE FROM run_artifact_upload_intents WHERE id = $1;

-- name: DeleteRunArtifactUploadIntentByObjectKey :exec
DELETE FROM run_artifact_upload_intents WHERE object_key = @object_key;

-- name: CountLiveRunArtifactsSharingObject :one
SELECT count(*) FROM artifacts
WHERE kind = 'run_output' AND object_key = @object_key
  AND deleted_at IS NULL AND purged_at IS NULL AND expires_at > now();

-- name: LockRunArtifactObjectKey :exec
SELECT pg_advisory_xact_lock(hashtextextended('artifact-object:' || @object_key::text, 0));
