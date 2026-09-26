-- name: ListDownloadArtifactsWithIdentity :many
SELECT da.artifact_id, da.skill_version_id, da.target, da.profile_version,
       da.packager_version, da.manifest_hash, da.includes_test_cases,
       a.file_name, a.size_bytes, a.content_hash, a.scan_status,
       a.expires_at, a.created_at, a.deleted_at, a.purged_at,
       (SELECT count(*) FROM download_records dr WHERE dr.artifact_id = da.artifact_id)::bigint
           AS download_count
FROM download_artifacts da
JOIN artifacts a ON a.id = da.artifact_id
WHERE da.workspace_id = $1
  AND da.skill_version_id = $2
  AND da.target = $3
  AND da.packager_version = $4
  AND da.includes_test_cases = $5
  AND a.content_hash = $6
ORDER BY a.created_at DESC;

-- name: CreateDownloadArtifactRow :one
INSERT INTO artifacts (
    workspace_id, run_id, kind, file_name, content_type,
    size_bytes, content_hash, object_key, expires_at
) VALUES ($1, NULL, 'download_package', $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: CreateDownloadArtifactDetail :exec
INSERT INTO download_artifacts (
    artifact_id, workspace_id, skill_version_id, target,
    profile_version, packager_version, manifest_hash, includes_test_cases
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: CreatePluginArtifactDetail :exec
INSERT INTO download_artifacts (
    artifact_id, workspace_id, plugin_name, plugin_version, target,
    profile_version, packager_version, manifest_hash, includes_test_cases
) VALUES (@artifact_id, @workspace_id, @plugin_name, @plugin_version, @target,
          @profile_version, @packager_version, @manifest_hash, false);

-- name: InsertDownloadArtifactMember :exec
INSERT INTO download_artifact_members (artifact_id, workspace_id, skill_version_id, position)
VALUES (@artifact_id, @workspace_id, @skill_version_id, @position);

-- name: ListPluginArtifactsWithIdentity :many
SELECT da.artifact_id, da.target, da.profile_version,
       da.packager_version, da.manifest_hash,
       a.file_name, a.size_bytes, a.content_hash, a.scan_status,
       a.expires_at, a.created_at, a.deleted_at, a.purged_at,
       (SELECT count(*) FROM download_records dr WHERE dr.artifact_id = da.artifact_id)::bigint
           AS download_count
FROM download_artifacts da
JOIN artifacts a ON a.id = da.artifact_id
WHERE da.workspace_id = @workspace_id
  AND da.plugin_name = @plugin_name
  AND da.plugin_version = @plugin_version
  AND da.packager_version = @packager_version
  AND a.content_hash = @content_hash
ORDER BY a.created_at DESC;

-- name: MarkDownloadArtifactAvailable :exec
UPDATE artifacts SET scan_status = 'available'
WHERE id = $1 AND workspace_id = $2 AND kind = 'download_package';

-- name: ListDownloadArtifacts :many
SELECT da.artifact_id, da.skill_version_id, da.target, da.profile_version,
       da.packager_version, da.manifest_hash, da.includes_test_cases,
       a.file_name, a.size_bytes, a.content_hash, a.scan_status,
       a.expires_at, a.created_at, a.purged_at,
       (SELECT count(*) FROM download_records dr WHERE dr.artifact_id = da.artifact_id)::bigint
           AS download_count,
       da.plugin_name, da.plugin_version,
       (SELECT coalesce(array_agg(m.skill_version_id ORDER BY m.position), '{}')
          FROM download_artifact_members m WHERE m.artifact_id = da.artifact_id)::uuid[]
           AS member_version_ids
FROM download_artifacts da
JOIN artifacts a ON a.id = da.artifact_id
WHERE da.workspace_id = $1 AND a.deleted_at IS NULL
ORDER BY a.created_at DESC, da.artifact_id;

-- name: GetDownloadArtifact :one
SELECT da.artifact_id, da.skill_version_id, da.target, da.profile_version,
       da.packager_version, da.manifest_hash, da.includes_test_cases,
       a.file_name, a.size_bytes, a.content_hash, a.scan_status, a.object_key,
       a.expires_at, a.created_at, a.purged_at,
       (SELECT count(*) FROM download_records dr WHERE dr.artifact_id = da.artifact_id)::bigint
           AS download_count,
       da.plugin_name, da.plugin_version,
       (SELECT coalesce(array_agg(m.skill_version_id ORDER BY m.position), '{}')
          FROM download_artifact_members m WHERE m.artifact_id = da.artifact_id)::uuid[]
           AS member_version_ids
FROM download_artifacts da
JOIN artifacts a ON a.id = da.artifact_id
WHERE da.workspace_id = $1 AND da.artifact_id = $2 AND a.deleted_at IS NULL;

-- name: InsertDownloadRecord :exec
INSERT INTO download_records (workspace_id, artifact_id, actor_user_id)
VALUES ($1, $2, $3);

-- name: ListDownloadRecordsForArtifact :many
SELECT dr.downloaded_at, dr.actor_user_id
FROM download_records dr
JOIN download_artifacts da ON da.artifact_id = dr.artifact_id
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

-- name: DeleteWorkspaceDownloadArtifactMembers :execrows
DELETE FROM download_artifact_members WHERE workspace_id = $1;

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
INSERT INTO download_object_cleanup_intents (workspace_id, object_key, not_before)
VALUES (@workspace_id, @object_key, now() + @hold::interval)
ON CONFLICT (object_key) DO UPDATE
SET workspace_id = excluded.workspace_id,
    not_before = excluded.not_before, attempted_at = NULL
RETURNING *;

-- name: DeleteDownloadCleanupIntent :exec
DELETE FROM download_object_cleanup_intents
WHERE object_key = @object_key AND workspace_id = @workspace_id;

-- name: LockPackagingWorkspaceObjectsSession :exec
SELECT pg_advisory_lock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: UnlockPackagingWorkspaceObjectsSession :one
SELECT pg_advisory_unlock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: LockDownloadObjectKeySession :exec
SELECT pg_advisory_lock(hashtextextended(@lock_key::text, 0));

-- name: UnlockDownloadObjectKeySession :one
SELECT pg_advisory_unlock(hashtextextended(@lock_key::text, 0));

-- name: ListSkillVersionsInDownloads :many
SELECT skill_version_id::uuid FROM download_artifacts WHERE skill_version_id = ANY(@version_ids::uuid[])
UNION
SELECT skill_version_id FROM download_artifact_members WHERE skill_version_id = ANY(@version_ids::uuid[]);
