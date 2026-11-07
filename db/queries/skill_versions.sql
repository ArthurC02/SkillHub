-- name: CreateSkillVersion :one
INSERT INTO skill_versions (
    workspace_id, skill_id, source_id, version_number,
    content_hash, package_object_key, source_path, manifest, license_expression, license_source,
    held_for_evaluation
) VALUES (
    @workspace_id, @skill_id, @source_id, @version_number,
    @content_hash, @package_object_key, @source_path, @manifest, @license_expression, @license_source,
    @held_for_evaluation
)
RETURNING *;

-- name: GetSkillVersion :one
SELECT * FROM skill_versions
WHERE skill_versions.id = $1 AND skill_versions.workspace_id = $2
  AND EXISTS (
      SELECT 1 FROM skills sk
      WHERE sk.id = skill_versions.skill_id AND sk.deleted_at IS NULL
  );

-- name: ListVersionSummaries :many
SELECT sv.id, sv.skill_id, sv.version_number, sk.name AS skill_name,
       sk.access_restriction, sk.redistribution, (sk.takedown_at IS NOT NULL)::bool AS taken_down,
       (SELECT max(v2.version_number) FROM skill_line_versions v2
         WHERE v2.skill_id = sv.skill_id)::int AS latest_version_number
FROM skill_versions sv
JOIN skills sk ON sk.id = sv.skill_id
WHERE sv.workspace_id = @workspace_id AND sv.id = ANY(@version_ids::uuid[]);

-- name: ListVersionSummariesByID :many
SELECT sv.id, sv.skill_id, sv.version_number, sk.name AS skill_name,
       sk.access_restriction, sk.redistribution, (sk.takedown_at IS NOT NULL)::bool AS taken_down,
       (SELECT max(v2.version_number) FROM skill_line_versions v2
         WHERE v2.skill_id = sv.skill_id)::int AS latest_version_number
FROM skill_versions sv
JOIN skills sk ON sk.id = sv.skill_id
WHERE sv.id = ANY(@version_ids::uuid[]);

-- name: ListSkillVersions :many
SELECT sqlc.embed(skill_versions),
       EXISTS (SELECT 1 FROM skill_version_promotions p WHERE p.version_id = skill_versions.id) AS promoted
FROM skill_versions
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

-- name: GetNewestSkillVersion :one
SELECT id, version_number, license_expression, license_source
FROM skill_line_versions
WHERE skill_id = $1
ORDER BY version_number DESC
LIMIT 1;

-- name: GetHighestVersionNumber :one
SELECT coalesce(max(version_number), 0)::int FROM skill_versions WHERE skill_id = @skill_id AND workspace_id = @workspace_id;

-- name: InsertSkillVersionPromotion :one
INSERT INTO skill_version_promotions (version_id, workspace_id, promoted_by)
VALUES (@version_id, @workspace_id, sqlc.narg(promoted_by))
ON CONFLICT (version_id) DO NOTHING
RETURNING *;

-- name: IsVersionInLine :one
SELECT EXISTS (SELECT 1 FROM skill_line_versions WHERE id = @version_id AND workspace_id = @workspace_id);

-- name: RememberPackageObject :exec
INSERT INTO object_collection_queue (object_key) VALUES ($1)
ON CONFLICT (object_key) DO NOTHING;

-- name: LockPackageObjectSession :exec
SELECT pg_advisory_lock(hashtextextended('package-object:' || sqlc.arg(object_key)::text, 0));

-- name: UnlockPackageObjectSession :one
SELECT pg_advisory_unlock(hashtextextended('package-object:' || sqlc.arg(object_key)::text, 0));

-- name: PackageObjectCollectable :one
SELECT NOT EXISTS (
    SELECT 1 FROM skill_versions WHERE package_object_key = sqlc.arg(object_key) AND content_erased_at IS NULL
);

-- name: ListSkillSourcesInVersions :many
SELECT DISTINCT source_id FROM skill_versions WHERE source_id = ANY(@source_ids::uuid[]);

-- name: ListSourcePaths :many
SELECT DISTINCT ON (source_id) source_id, source_path
FROM skill_versions
WHERE source_id = ANY(@source_ids::uuid[])
ORDER BY source_id, version_number DESC;

-- name: OldestCollectableObjectEnqueuedAt :one
SELECT min(enqueued_at)::timestamptz FROM object_collection_queue;
