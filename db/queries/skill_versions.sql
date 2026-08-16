-- name: CreateSkillVersion :one
-- version_number is allocated inline; concurrent saves lose on skill_versions_number_key
-- and the caller retries. Existing versions are never overwritten (WS-001, iron rule 4).
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
WHERE id = $1 AND workspace_id = $2;

-- name: ListSkillVersions :many
SELECT * FROM skill_versions
WHERE workspace_id = $1 AND skill_id = $2
ORDER BY version_number DESC;

-- name: GetSkillRuntimeCompatibility :one
-- The newest measurement for this version, on whatever runtime image it was made
-- (0022). No workspace scope: the row hangs off a version the caller has already
-- been authorised to read, and the measurement itself carries no user content.
--
-- Newest-wins rather than newest-per-image: the detail view has one compatibility
-- block, and a list of "on this image X, on that image Y" is a question nobody
-- asked at this stage. The image the answer came from travels with it, so the
-- reader can tell what was actually measured.
SELECT capability, runtime, runtime_image, measured_at
FROM skill_runtime_compatibility
WHERE skill_version_id = $1
ORDER BY measured_at DESC
LIMIT 1;
