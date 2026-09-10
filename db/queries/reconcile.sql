-- name: ListArtifactsPastRetention :many
WITH candidates AS (
    SELECT id FROM artifacts
    WHERE kind = 'download_package'
      AND purged_at IS NULL
	  AND (deleted_at IS NOT NULL OR expires_at <= now())
	  AND (retention_attempted_at IS NULL OR retention_attempted_at < now() - interval '15 minutes')
    ORDER BY retention_attempted_at NULLS FIRST, retention_attempted_at, expires_at, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE artifacts a SET retention_attempted_at = now()
FROM candidates c WHERE a.id = c.id
RETURNING a.id, a.workspace_id, a.object_key;

-- name: MarkArtifactPurged :exec
WITH cleared_sighting AS (
    DELETE FROM object_reconcile_sightings
    WHERE resource_kind = 'artifact' AND resource_id = $1
)
UPDATE artifacts SET purged_at = now()
WHERE id = $1 AND kind = 'download_package' AND purged_at IS NULL;

-- name: ListRunOutputsPastRetention :many
WITH candidates AS (
    SELECT id FROM artifacts
    WHERE kind = 'run_output'
      AND purged_at IS NULL
	  AND (deleted_at IS NOT NULL OR expires_at <= now())
	  AND (retention_attempted_at IS NULL OR retention_attempted_at < now() - interval '15 minutes')
    ORDER BY retention_attempted_at NULLS FIRST, retention_attempted_at, expires_at, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE artifacts a SET retention_attempted_at = now()
FROM candidates c WHERE a.id = c.id
RETURNING a.id, a.workspace_id, a.object_key;

-- name: MarkRunOutputPurged :exec
WITH cleared_sighting AS (
    DELETE FROM object_reconcile_sightings
    WHERE resource_kind = 'artifact' AND resource_id = $1
)
UPDATE artifacts SET purged_at = now()
WHERE id = $1 AND kind = 'run_output' AND purged_at IS NULL;

-- name: ListArtifactsClaimingObject :many
WITH candidates AS (
    SELECT id FROM artifacts
    WHERE kind = 'download_package'
      AND scan_status = 'available'
      AND deleted_at IS NULL
      AND purged_at IS NULL
	  AND expires_at > now()
	  AND (reconcile_checked_at IS NULL OR reconcile_checked_at < now() - interval '15 minutes')
	ORDER BY reconcile_checked_at NULLS FIRST, reconcile_checked_at, created_at, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE artifacts a SET reconcile_checked_at = now()
FROM candidates c WHERE a.id = c.id
RETURNING a.id, a.workspace_id, a.object_key;

-- name: ListDatasetsClaimingObject :many
WITH candidates AS (
    SELECT id FROM datasets
    WHERE deleted_at IS NULL
      AND purged_at IS NULL
      AND expires_at > now()
	  AND (reconcile_checked_at IS NULL OR reconcile_checked_at < now() - interval '15 minutes')
    ORDER BY reconcile_checked_at NULLS FIRST, reconcile_checked_at, created_at, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE datasets d SET reconcile_checked_at = now()
FROM candidates c WHERE d.id = c.id
RETURNING d.id, d.workspace_id, d.object_key;

-- name: ListDatasetsPastRetention :many
WITH candidates AS (
    SELECT id FROM datasets
    WHERE purged_at IS NULL
      AND (deleted_at IS NOT NULL OR expires_at <= now())
	  AND (retention_attempted_at IS NULL OR retention_attempted_at < now() - interval '15 minutes')
    ORDER BY retention_attempted_at NULLS FIRST, retention_attempted_at, expires_at, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE datasets d SET retention_attempted_at = now()
FROM candidates c WHERE d.id = c.id
RETURNING d.id, d.workspace_id, d.object_key;

-- name: MarkDatasetPurged :exec
WITH cleared_sighting AS (
    DELETE FROM object_reconcile_sightings
    WHERE resource_kind = 'dataset' AND resource_id = $1
)
UPDATE datasets SET deleted_at = coalesce(deleted_at, now()), purged_at = now()
WHERE id = $1 AND purged_at IS NULL;

-- name: ListDatasetCleanupIntents :many
WITH candidates AS (
    SELECT id FROM dataset_object_cleanup_intents
    WHERE not_before <= now()
	  AND (attempted_at IS NULL OR attempted_at < now() - interval '15 minutes')
    ORDER BY attempted_at NULLS FIRST, attempted_at, not_before, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE dataset_object_cleanup_intents i SET attempted_at = now()
FROM candidates c WHERE i.id = c.id
RETURNING i.id, i.workspace_id, i.object_key;

-- name: MarkDatasetCleanupIntentPurged :exec
DELETE FROM dataset_object_cleanup_intents WHERE id = $1;

-- name: ListDownloadCleanupIntents :many
WITH candidates AS (
    SELECT id FROM download_object_cleanup_intents
    WHERE not_before <= now()
      AND (attempted_at IS NULL OR attempted_at < now() - interval '15 minutes')
    ORDER BY attempted_at NULLS FIRST, attempted_at, not_before, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE download_object_cleanup_intents i SET attempted_at = now()
FROM candidates c WHERE i.id = c.id
RETURNING i.id, i.workspace_id, i.object_key;

-- name: MarkDownloadCleanupIntentPurged :exec
DELETE FROM download_object_cleanup_intents WHERE id = $1;

-- name: LockDatasetObjectKey :exec
SELECT pg_advisory_xact_lock(hashtextextended('dataset-object:' || @object_key::text, 0));

-- name: CountLiveDatasetsSharingObject :one
SELECT count(*) FROM datasets
WHERE object_key = @object_key AND deleted_at IS NULL AND purged_at IS NULL
  AND expires_at > now();

-- name: RecordObjectSighting :one
-- Returns the consecutive-round count; callers act only from round two.
INSERT INTO object_reconcile_sightings (resource_kind, resource_id, object_key)
VALUES ($1, $2, $3)
ON CONFLICT (resource_kind, resource_id) DO UPDATE
SET rounds = object_reconcile_sightings.rounds + 1, last_seen_at = now()
RETURNING rounds;

-- name: ClearObjectSighting :exec
-- Deleting on each clear keeps rounds consecutive rather than cumulative.
DELETE FROM object_reconcile_sightings
WHERE resource_kind = $1 AND resource_id = $2;

-- name: CountPersistentObjectSightings :many
SELECT resource_kind, count(*)::bigint AS sightings
FROM object_reconcile_sightings
WHERE rounds >= $1
GROUP BY resource_kind;

-- name: MarkDatasetObjectLost :exec
UPDATE datasets SET deleted_at = coalesce(deleted_at, now()), purged_at = now()
WHERE id = $1 AND purged_at IS NULL;
