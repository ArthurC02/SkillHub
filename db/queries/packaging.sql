-- name: FindReusableDownloadArtifact :one
SELECT da.artifact_id, da.skill_version_id, da.target, da.profile_version,
       da.packager_version, da.manifest_hash, da.includes_test_cases,
       sv.version_number,
       (SELECT max(v2.version_number) FROM skill_versions v2
         WHERE v2.skill_id = sv.skill_id)::int AS latest_version_number,
       a.file_name, a.size_bytes, a.content_hash, a.scan_status,
       a.expires_at, a.created_at,
       (SELECT count(*) FROM download_records dr WHERE dr.artifact_id = da.artifact_id)::bigint
           AS download_count
FROM download_artifacts da
JOIN artifacts a ON a.id = da.artifact_id
JOIN skill_versions sv ON sv.id = da.skill_version_id
WHERE da.workspace_id = $1
  AND da.skill_version_id = $2
  AND da.target = $3
  AND da.packager_version = $4
  AND da.includes_test_cases = $5
  AND a.content_hash = $6
  AND a.scan_status = 'available'
  AND a.deleted_at IS NULL
  AND a.purged_at IS NULL
  AND a.expires_at > now()
ORDER BY a.created_at DESC
LIMIT 1;

-- name: CreateDownloadArtifactRow :one
INSERT INTO artifacts (
    workspace_id, run_id, kind, file_name, content_type,
    size_bytes, content_hash, object_key, expires_at
) VALUES ($1, NULL, 'download_package', $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: CreateDownloadArtifactDetail :one
INSERT INTO download_artifacts (
    artifact_id, workspace_id, skill_version_id, target,
    profile_version, packager_version, manifest_hash, includes_test_cases
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *, (
    SELECT max(v2.version_number) FROM skill_versions v2
     WHERE v2.skill_id = (SELECT v1.skill_id FROM skill_versions v1
                           WHERE v1.id = download_artifacts.skill_version_id)
)::int AS latest_version_number;

-- name: MarkDownloadArtifactAvailable :exec
UPDATE artifacts SET scan_status = 'available'
WHERE id = $1 AND workspace_id = $2 AND kind = 'download_package';

-- name: ListDownloadArtifacts :many
SELECT da.artifact_id, da.skill_version_id, da.target, da.profile_version,
       da.packager_version, da.manifest_hash, da.includes_test_cases,
       sv.skill_id,
       sv.version_number,
       (SELECT max(v2.version_number) FROM skill_versions v2
         WHERE v2.skill_id = sv.skill_id)::int AS latest_version_number,
       a.file_name, a.size_bytes, a.content_hash, a.scan_status,
       a.expires_at, a.created_at, a.purged_at,
       (SELECT count(*) FROM download_records dr WHERE dr.artifact_id = da.artifact_id)::bigint
           AS download_count
FROM download_artifacts da
JOIN artifacts a ON a.id = da.artifact_id
JOIN skill_versions sv ON sv.id = da.skill_version_id
WHERE da.workspace_id = $1 AND a.deleted_at IS NULL
ORDER BY a.created_at DESC, da.artifact_id;

-- name: GetDownloadArtifact :one
SELECT da.artifact_id, da.skill_version_id, da.target, da.profile_version,
       da.packager_version, da.manifest_hash, da.includes_test_cases,
       sv.skill_id,
       sv.version_number,
       (SELECT max(v2.version_number) FROM skill_versions v2
         WHERE v2.skill_id = sv.skill_id)::int AS latest_version_number,
       a.file_name, a.size_bytes, a.content_hash, a.scan_status, a.object_key,
       a.expires_at, a.created_at, a.purged_at,
       sk.access_restriction, sk.redistribution,
       (SELECT count(*) FROM download_records dr WHERE dr.artifact_id = da.artifact_id)::bigint
           AS download_count
FROM download_artifacts da
JOIN artifacts a ON a.id = da.artifact_id
JOIN skill_versions sv ON sv.id = da.skill_version_id
JOIN skills sk ON sk.id = sv.skill_id
WHERE da.workspace_id = $1 AND da.artifact_id = $2 AND a.deleted_at IS NULL;

-- name: InsertDownloadRecord :exec
INSERT INTO download_records (workspace_id, artifact_id, actor_user_id)
VALUES ($1, $2, $3);

-- name: ListDownloadRecordsForArtifact :many
SELECT dr.downloaded_at, dr.actor_user_id, u.display_name
FROM download_records dr
JOIN download_artifacts da ON da.artifact_id = dr.artifact_id
LEFT JOIN users u ON u.id = dr.actor_user_id
WHERE dr.workspace_id = @workspace_id AND dr.artifact_id = @artifact_id
  AND da.workspace_id = @workspace_id
ORDER BY dr.downloaded_at DESC;

-- name: SoftDeleteDownloadArtifact :one
UPDATE artifacts SET deleted_at = now()
WHERE id = $1 AND workspace_id = $2 AND kind = 'download_package' AND deleted_at IS NULL
RETURNING id, object_key, purged_at;

-- name: GetDownloadArtifactForDelete :one
SELECT id, object_key, purged_at FROM artifacts
WHERE id = $1 AND workspace_id = $2 AND kind = 'download_package' AND deleted_at IS NULL;

-- name: DeleteWorkspaceDownloadRecords :execrows
DELETE FROM download_records WHERE workspace_id = $1;

-- name: DeleteWorkspaceDownloadArtifactDetails :execrows
DELETE FROM download_artifacts WHERE workspace_id = $1;

-- name: CountArtifactsSharingObject :one
SELECT count(*)::bigint FROM artifacts
WHERE object_key = $1 AND deleted_at IS NULL AND purged_at IS NULL
  AND expires_at > now();

-- name: ListTestCasesForSkill :many
SELECT * FROM test_cases
WHERE skill_id = $1 AND workspace_id = $2 AND deleted_at IS NULL
ORDER BY created_at, id;

-- name: ListSuggestionsAppliedToVersion :many
SELECT evaluation_id, category, target_path
FROM evaluation_suggestions
WHERE applied_skill_version_id = $1 AND workspace_id = $2
ORDER BY created_at, id;

-- name: GetPreviousSkillVersion :one
SELECT id, skill_id, version_number FROM skill_versions
WHERE skill_id = $1 AND workspace_id = $2 AND version_number < $3
ORDER BY version_number DESC
LIMIT 1;

-- name: GetVersionLineage :one
SELECT sv.id, sv.skill_id, sv.version_number, sv.source_id,
       sk.forked_from_skill_id, sk.forked_from_version_id
FROM skill_versions sv
JOIN skills sk ON sk.id = sv.skill_id
WHERE sv.id = $1;

-- name: GetOldestSkillVersion :one
SELECT id, skill_id, version_number, source_id
FROM skill_versions
WHERE skill_id = $1
ORDER BY version_number
LIMIT 1;

-- name: GetLineageSource :one
SELECT source_type, source_url, source_ref, content_hash, fetched_at
FROM skill_sources
WHERE id = $1;
-- name: LockDownloadObjectKey :exec
SELECT pg_advisory_xact_lock(hashtextextended(@lock_key::text, 0));

-- name: CreateDownloadCleanupIntent :one
INSERT INTO download_object_cleanup_intents (workspace_id, object_key)
VALUES (@workspace_id, @object_key)
ON CONFLICT (object_key) DO UPDATE
SET workspace_id = excluded.workspace_id,
    not_before = now() + interval '1 hour', attempted_at = NULL
RETURNING *;

-- name: DeleteDownloadCleanupIntent :exec
DELETE FROM download_object_cleanup_intents
WHERE object_key = @object_key AND workspace_id = @workspace_id;

-- name: LockPackagingWorkspaceObjects :exec
SELECT pg_advisory_xact_lock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: LockPackagingWorkspaceObjectsSession :exec
SELECT pg_advisory_lock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: UnlockPackagingWorkspaceObjectsSession :one
SELECT pg_advisory_unlock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: LockDownloadObjectKeySession :exec
SELECT pg_advisory_lock(hashtextextended(@lock_key::text, 0));

-- name: UnlockDownloadObjectKeySession :one
SELECT pg_advisory_unlock(hashtextextended(@lock_key::text, 0));
