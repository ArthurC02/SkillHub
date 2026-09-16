-- name: CreateRun :one
INSERT INTO runs (
    workspace_id, skill_version_id, test_case_snapshot_id, provider,
    runtime_snapshot, policy_snapshot, status
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetRun :one
SELECT * FROM runs WHERE id = $1 AND workspace_id = $2;

-- name: LockRun :one
SELECT * FROM runs WHERE id = $1 AND workspace_id = $2 FOR UPDATE;

-- name: ListWorkspaceRuns :many
SELECT r.id, r.status, r.status_reason, r.provider, r.failure_class,
       r.cleanup_status, r.skill_version_id, r.test_case_snapshot_id,
       r.cancel_requested_at, r.created_at, r.started_at, r.finished_at
FROM runs r
WHERE r.workspace_id = @workspace_id
  AND (sqlc.narg(snapshot_ids)::uuid[] IS NULL OR r.test_case_snapshot_id = ANY(sqlc.narg(snapshot_ids)::uuid[]))
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
SELECT skill_version_id, test_case_snapshot_id
FROM runs
WHERE id = @run_id AND workspace_id = @workspace_id;

-- name: TransitionRun :one
UPDATE runs SET
    status = @to_status,
    status_reason = @reason,
    failure_class = @failure_class,
    started_at = @started_at,
    finished_at = @finished_at
WHERE id = @run_id AND workspace_id = @workspace_id AND status = @from_status
RETURNING *;

-- name: RequestRunCancel :one
UPDATE runs
SET cancel_requested_at = @cancel_requested_at
WHERE id = @id AND workspace_id = @workspace_id AND status = @status
RETURNING *;

-- name: SetRunProvider :one
UPDATE runs SET provider = @provider, runtime_snapshot = @runtime_snapshot
WHERE id = @id AND workspace_id = @workspace_id AND status = @status
RETURNING *;

-- name: ListActiveRuns :many
WITH candidates AS (
    SELECT id FROM runs
    WHERE status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out')
	  AND (supervision_checked_at IS NULL OR supervision_checked_at < now() - @recheck_after::interval)
    ORDER BY supervision_checked_at NULLS FIRST, supervision_checked_at, created_at, id
    LIMIT @batch_size FOR UPDATE SKIP LOCKED
)
UPDATE runs r SET supervision_checked_at = now()
FROM candidates c WHERE r.id = c.id
RETURNING r.*;

-- name: ListRunsNeedingCleanup :many
WITH candidates AS (
    SELECT id FROM runs
    WHERE status IN ('succeeded', 'failed', 'cancelled', 'timed_out')
      AND cleanup_status <> 'cleaned'
      AND finished_at < now() - @settled_for::interval
	  AND (cleanup_attempted_at IS NULL OR cleanup_attempted_at < now() - @recheck_after::interval)
    ORDER BY cleanup_attempted_at NULLS FIRST, cleanup_attempted_at, finished_at, id
    LIMIT @batch_size FOR UPDATE SKIP LOCKED
)
UPDATE runs r SET cleanup_attempted_at = now()
FROM candidates c WHERE r.id = c.id
RETURNING r.*;

-- name: SetRunCleanupStatus :one
UPDATE runs SET
    cleanup_status = @cleanup_status,
    cleanup_at = coalesce(sqlc.narg(settled_at), cleanup_at)
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
INSERT INTO run_attempts (
    run_id, workspace_id, attempt_number, provider, object_grants_state, object_grants_expire_at
) VALUES (
    @run_id, @workspace_id, @attempt_number, @provider, @object_grants_state, @object_grants_expire_at
)
RETURNING *;

-- name: SetAttemptProviderRunID :one
UPDATE run_attempts SET provider_run_id = @provider_run_id, started_at = @started_at
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: FinishRunAttempt :one
UPDATE run_attempts
SET finished_at = @finished_at, error_class = @error_class, error_message = @error_message,
    object_grants_state = @object_grants_state, object_grants_expire_at = @object_grants_expire_at
WHERE id = @id AND workspace_id = @workspace_id
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

-- name: InsertRunArtifact :exec
INSERT INTO artifacts (
    workspace_id, run_id, kind, file_name, content_type, size_bytes, content_hash,
    object_key, expires_at
) VALUES (
    @workspace_id, @run_id, 'run_output', @file_name, @content_type, @size_bytes,
    @content_hash, @object_key, @expires_at
);

-- name: ListRunArtifactFileNames :many
SELECT file_name FROM artifacts
WHERE run_id = @run_id AND workspace_id = @workspace_id AND kind = 'run_output';

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
WHERE provider = @provider AND rounds >= @persistent_after_rounds::int;
-- name: AccountPurgeReady :one
SELECT NOT EXISTS (
    SELECT 1 FROM runs r
    WHERE r.workspace_id = @workspace_id
      AND (r.status::text <> ALL(@terminal_statuses::text[])
           OR r.cleanup_status::text <> @settled_cleanup_status::text
           OR EXISTS (
               SELECT 1 FROM run_attempts a
               WHERE a.run_id = r.id
                 AND (a.object_grants_state = ANY(@unprovable_grant_states::text[])
                      OR a.object_grants_expire_at > now() - @clock_tolerance::interval)
           ))
);

-- name: SetRunAttemptObjectGrants :execrows
UPDATE run_attempts
SET object_grants_state = @object_grants_state, object_grants_expire_at = @object_grants_expire_at
WHERE id = @id AND workspace_id = @workspace_id;

-- name: ListRunArtifactUploadIntents :many
WITH candidates AS (
    SELECT id FROM run_artifact_upload_intents
    WHERE not_before <= now()
      AND (attempted_at IS NULL OR attempted_at < now() - @claim_lease::interval)
    ORDER BY attempted_at NULLS FIRST, attempted_at, not_before, id
    LIMIT @batch_size FOR UPDATE SKIP LOCKED
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

-- name: ListSkillVersionsInRuns :many
SELECT DISTINCT skill_version_id FROM runs WHERE skill_version_id = ANY(@version_ids::uuid[]);

-- name: CountRunsByDay :many
SELECT (created_at AT TIME ZONE 'UTC')::date AS day, status::text AS status, count(*)::bigint AS runs
FROM runs
WHERE created_at >= @since::timestamptz
GROUP BY 1, 2
ORDER BY 1, 2;
