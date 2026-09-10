-- name: CreateSkillVersion :one
INSERT INTO skill_versions (
    workspace_id, skill_id, source_id, version_number,
    content_hash, package_object_key, manifest, license_expression, license_source
) VALUES (
    $1, $2, $3,
    (SELECT coalesce(max(version_number), 0) + 1 FROM skill_versions WHERE skill_id = $2),
    $4, $5, $6, $7, $8
)
RETURNING *;

-- name: GetSkillVersion :one
SELECT * FROM skill_versions
WHERE skill_versions.id = $1 AND skill_versions.workspace_id = $2
  AND EXISTS (
      SELECT 1 FROM skills sk
      WHERE sk.id = skill_versions.skill_id AND sk.deleted_at IS NULL
  );

-- name: ListSkillVersions :many
SELECT * FROM skill_versions
WHERE skill_versions.workspace_id = $1 AND skill_versions.skill_id = $2
  AND EXISTS (
      SELECT 1 FROM skills sk
      WHERE sk.id = skill_versions.skill_id AND sk.deleted_at IS NULL
  )
ORDER BY version_number DESC;

-- name: GetSkillRuntimeCompatibility :one
SELECT capability, runtime, runtime_image, measured_at
FROM skill_runtime_compatibility
WHERE skill_version_id = $1
ORDER BY measured_at DESC
LIMIT 1;

-- name: GetLatestVersionLicense :one
SELECT license_expression, license_source
FROM skill_versions
WHERE skill_id = $1
ORDER BY version_number DESC
LIMIT 1;
-- name: RememberPackageObject :exec
INSERT INTO object_collection_queue (object_key) VALUES ($1)
ON CONFLICT (object_key) DO NOTHING;

-- name: LockPackageObjectSession :exec
SELECT pg_advisory_lock(hashtextextended('package-object:' || sqlc.arg(object_key)::text, 0));

-- name: UnlockPackageObjectSession :one
SELECT pg_advisory_unlock(hashtextextended('package-object:' || sqlc.arg(object_key)::text, 0));

-- name: PackageObjectCollectable :one
SELECT NOT EXISTS (
    SELECT 1 FROM skill_versions WHERE package_object_key = sqlc.arg(object_key)
);
