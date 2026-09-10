-- name: InsertAuditEvent :exec
INSERT INTO audit_events (actor_user_id, workspace_id, action, resource_type, resource_id, metadata)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListWorkspaceAuditEvents :many
SELECT * FROM audit_events
WHERE workspace_id = $1 AND action = ANY(@actions::text[])
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: DeleteExpiredAuditEvents :execrows
DELETE FROM audit_events WHERE created_at < $1;

-- name: RequestAccountDeletion :one
UPDATE users
SET deletion_requested_at = coalesce(deletion_requested_at, now()),
    purge_attempted_at = CASE WHEN deletion_requested_at IS NULL THEN NULL ELSE purge_attempted_at END,
    purge_started_at = CASE WHEN deletion_requested_at IS NULL THEN NULL ELSE purge_started_at END,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL AND purge_started_at IS NULL
RETURNING *;

-- name: CancelAccountDeletion :one
UPDATE users SET deletion_requested_at = NULL, purge_attempted_at = NULL,
    purge_started_at = NULL, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL AND purge_started_at IS NULL
RETURNING *;

-- name: ListAccountsPastGrace :many
WITH candidates AS (
    SELECT pending.id FROM users pending
    WHERE pending.deleted_at IS NULL
      AND pending.deletion_requested_at IS NOT NULL
      AND pending.deletion_requested_at <= sqlc.arg(cutoff)
	  AND (pending.purge_attempted_at IS NULL OR pending.purge_attempted_at < now() - interval '15 minutes')
    ORDER BY pending.purge_attempted_at NULLS FIRST, pending.purge_attempted_at,
             pending.deletion_requested_at, pending.id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE users u SET purge_attempted_at = now()
FROM candidates c WHERE u.id = c.id
RETURNING u.id;

-- name: MarkAccountPurgeStarted :execrows
UPDATE users SET purge_started_at = coalesce(purge_started_at, now()), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL AND deletion_requested_at IS NOT NULL;

-- name: ListWorkspaceDatasetObjectKeys :many
SELECT d.object_key FROM datasets d WHERE d.workspace_id = $1
UNION
SELECT i.object_key FROM dataset_object_cleanup_intents i WHERE i.workspace_id = $1;

-- name: LockAccountWorkspaceObjects :exec
SELECT pg_advisory_lock(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: UnlockAccountWorkspaceObjects :one
SELECT pg_advisory_unlock(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: ListWorkspaceRunArtifactObjectKeys :many
SELECT object_key FROM artifacts
WHERE artifacts.workspace_id = sqlc.arg(workspace_id)::uuid AND kind = 'run_output'
UNION
SELECT 'run-artifacts/' || r.id::text || '/' || a.id::text || '/artifacts.tar'
FROM runs r
JOIN run_attempts a ON a.run_id = r.id AND a.workspace_id = r.workspace_id
WHERE r.workspace_id = sqlc.arg(workspace_id)::uuid
UNION
SELECT object_key FROM run_artifact_upload_intents
WHERE workspace_id = sqlc.arg(workspace_id)::uuid;

-- name: ListWorkspaceDownloadArtifactObjectKeys :many
SELECT object_key FROM artifacts
WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND kind = 'download_package'
UNION
SELECT object_key FROM download_object_cleanup_intents
WHERE workspace_id = sqlc.arg(workspace_id)::uuid;

-- name: DeleteWorkspaceDatasets :execrows
WITH cleanup_intents AS (
    DELETE FROM dataset_object_cleanup_intents i WHERE i.workspace_id = $1
), sightings AS (
    DELETE FROM object_reconcile_sightings s USING datasets d
    WHERE s.resource_kind = 'dataset' AND s.resource_id = d.id AND d.workspace_id = $1
)
DELETE FROM datasets d WHERE d.workspace_id = $1;

-- name: DeleteWorkspaceTestCases :execrows
DELETE FROM test_cases
WHERE test_cases.workspace_id = $1
  AND NOT EXISTS (
      SELECT 1 FROM test_case_snapshots s WHERE s.test_case_id = test_cases.id
  );

-- name: DeleteWorkspaceRunArtifacts :execrows
WITH cleanup_intents AS (
    DELETE FROM run_artifact_upload_intents
    WHERE workspace_id = sqlc.arg(workspace_id)::uuid
), sightings AS (
    DELETE FROM object_reconcile_sightings s USING artifacts a
    WHERE s.resource_kind = 'artifact' AND s.resource_id = a.id
      AND a.workspace_id = sqlc.arg(workspace_id)::uuid AND a.kind = 'run_output'
)
DELETE FROM artifacts
WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND kind = 'run_output';

-- name: DeleteWorkspaceDownloadArtifacts :execrows
WITH cleanup_intents AS (
    DELETE FROM download_object_cleanup_intents
    WHERE workspace_id = sqlc.arg(workspace_id)::uuid
), sightings AS (
    DELETE FROM object_reconcile_sightings s USING artifacts a
    WHERE s.resource_kind = 'artifact' AND s.resource_id = a.id
      AND a.workspace_id = sqlc.arg(workspace_id)::uuid AND a.kind = 'download_package'
)
DELETE FROM artifacts
WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND kind = 'download_package';

-- name: PurgeUnreferencedSkills :execrows
WITH referenced AS (
    SELECT DISTINCT v.skill_id
    FROM skill_versions v
    WHERE EXISTS (
            SELECT 1 FROM skills f
            WHERE f.forked_from_version_id = v.id AND f.workspace_id <> v.workspace_id
          )
       OR EXISTS (SELECT 1 FROM runs r WHERE r.skill_version_id = v.id)
),
purgeable AS (
    SELECT sk.id FROM skills sk
    WHERE sk.workspace_id = $1
      AND NOT EXISTS (SELECT 1 FROM referenced ref WHERE ref.skill_id = sk.id)
      AND NOT EXISTS (
            SELECT 1 FROM skills f WHERE f.forked_from_skill_id = sk.id
          )
      AND NOT EXISTS (SELECT 1 FROM test_cases tc WHERE tc.skill_id = sk.id)
      AND NOT EXISTS (
            SELECT 1 FROM skills f
            JOIN skill_versions v ON v.id = f.forked_from_version_id
            WHERE v.skill_id = sk.id
          )
      AND NOT EXISTS (
            SELECT 1 FROM download_artifacts da
            JOIN skill_versions v ON v.id = da.skill_version_id
            WHERE v.skill_id = sk.id
          )
),
enqueued AS (
    INSERT INTO object_collection_queue (object_key)
    SELECT DISTINCT v.package_object_key
    FROM skill_versions v
    WHERE v.skill_id IN (SELECT id FROM purgeable)
      AND v.package_object_key <> ''
    ON CONFLICT (object_key) DO NOTHING
),
versions AS (
    DELETE FROM skill_versions WHERE skill_id IN (SELECT id FROM purgeable)
)
DELETE FROM skills WHERE id IN (SELECT id FROM purgeable);

-- name: PurgeSkillsPastDeletionGrace :execrows
WITH referenced AS (
    SELECT DISTINCT v.skill_id
    FROM skill_versions v
    WHERE EXISTS (
            SELECT 1 FROM skills f
            WHERE f.forked_from_version_id = v.id AND f.workspace_id <> v.workspace_id
          )
       OR EXISTS (SELECT 1 FROM runs r WHERE r.skill_version_id = v.id)
),
purgeable AS (
    SELECT sk.id FROM skills sk
    WHERE sk.deleted_at IS NOT NULL
      AND sk.deleted_at <= @cutoff::timestamptz
      AND NOT EXISTS (SELECT 1 FROM referenced ref WHERE ref.skill_id = sk.id)
      AND NOT EXISTS (
            SELECT 1 FROM skills f WHERE f.forked_from_skill_id = sk.id
          )
      AND NOT EXISTS (SELECT 1 FROM test_cases tc WHERE tc.skill_id = sk.id)
      AND NOT EXISTS (
            SELECT 1 FROM skills f
            JOIN skill_versions v ON v.id = f.forked_from_version_id
            WHERE v.skill_id = sk.id
          )
      AND NOT EXISTS (
            SELECT 1 FROM download_artifacts da
            JOIN skill_versions v ON v.id = da.skill_version_id
            WHERE v.skill_id = sk.id
          )
    ORDER BY sk.deleted_at
    LIMIT @row_limit::int
),
enqueued AS (
    INSERT INTO object_collection_queue (object_key)
    SELECT DISTINCT v.package_object_key
    FROM skill_versions v
    WHERE v.skill_id IN (SELECT id FROM purgeable)
      AND v.package_object_key <> ''
    ON CONFLICT (object_key) DO NOTHING
),
versions AS (
    DELETE FROM skill_versions WHERE skill_id IN (SELECT id FROM purgeable)
)
DELETE FROM skills WHERE id IN (SELECT id FROM purgeable);

-- name: CountSkillsAwaitingDeletionGrace :one
SELECT
    count(*) FILTER (
        WHERE sk.deleted_at > @cutoff::timestamptz
    )::bigint AS waiting,
    count(*) FILTER (
        WHERE sk.deleted_at <= @cutoff::timestamptz
          AND (
            EXISTS (SELECT 1 FROM skill_versions v
                    WHERE v.skill_id = sk.id
                      AND (EXISTS (SELECT 1 FROM skills f
                                   WHERE f.forked_from_version_id = v.id)
                           OR EXISTS (SELECT 1 FROM runs r WHERE r.skill_version_id = v.id)
                           OR EXISTS (SELECT 1 FROM download_artifacts da
                                      WHERE da.skill_version_id = v.id)))
            OR EXISTS (SELECT 1 FROM skills f WHERE f.forked_from_skill_id = sk.id)
            OR EXISTS (SELECT 1 FROM test_cases tc WHERE tc.skill_id = sk.id)
          )
    )::bigint AS kept
FROM skills sk
WHERE sk.deleted_at IS NOT NULL;

-- name: ListCollectableObjects :many
SELECT object_key FROM object_collection_queue q
WHERE NOT EXISTS (
    SELECT 1 FROM skill_versions v WHERE v.package_object_key = q.object_key
)
ORDER BY enqueued_at
LIMIT @row_limit::int;

-- name: DeleteObjectCollectionEntry :exec
DELETE FROM object_collection_queue WHERE object_key = $1;

-- name: DropReferencedCollectionEntries :execrows
DELETE FROM object_collection_queue q
WHERE EXISTS (
    SELECT 1 FROM skill_versions v WHERE v.package_object_key = q.object_key
);

-- name: CountCollectableObjects :one
SELECT count(*)::bigint FROM object_collection_queue;

-- name: PurgeUnreferencedSkillSources :execrows
DELETE FROM skill_sources s
WHERE s.workspace_id = $1
  AND NOT EXISTS (SELECT 1 FROM skill_versions v WHERE v.source_id = s.id);

-- name: DeleteUserIdentities :execrows
DELETE FROM user_identities WHERE user_id = $1;

-- name: DeleteUserSessions :execrows
DELETE FROM sessions WHERE user_id = $1;

-- name: AnonymizeWorkspacesByOwner :execrows
UPDATE workspaces SET name = 'deleted-workspace', updated_at = now()
WHERE owner_user_id = $1;

-- name: AnonymizeUser :one
UPDATE users
SET email = 'deleted-' || id::text || '@deleted.invalid',
    display_name = 'Deleted user',
    deleted_at = now(),
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: TakedownSkill :one
UPDATE skills
SET takedown_at = now(), takedown_reason = sqlc.arg(reason), updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL AND takedown_at IS NULL
RETURNING *;

-- name: LockSkillForOperatorWrite :one
SELECT id, workspace_id, access_restriction, redistribution, takedown_at FROM skills
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: SetSkillRedistribution :exec
UPDATE skills SET redistribution = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SetSkillAccessRestriction :exec
UPDATE skills SET access_restriction = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SetSkillTakedown :exec
UPDATE skills SET takedown_at = now(), takedown_reason = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL AND takedown_at IS NULL;

-- name: ListSourcesToCheck :many
SELECT id, workspace_id, source_url, unavailable_since, content_hash, content_changed_at
FROM skill_sources
WHERE source_type = 'git' AND source_url IS NOT NULL
ORDER BY last_checked_at NULLS FIRST
LIMIT $1;

-- name: MarkSourceChecked :exec
UPDATE skill_sources
SET last_checked_at = now(),
    unavailable_since = CASE
        WHEN sqlc.arg(available)::bool THEN NULL
        ELSE coalesce(unavailable_since, now())
    END,
    content_changed_at = CASE
        WHEN sqlc.arg(content_changed)::bool THEN coalesce(content_changed_at, now())
        ELSE content_changed_at
    END
WHERE id = $1;
