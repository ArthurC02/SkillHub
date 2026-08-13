-- name: CreateSkillVersion :one
-- version_number is allocated inline; concurrent saves lose on skill_versions_number_key
-- and the caller retries. Existing versions are never overwritten (WS-001, iron rule 4).
INSERT INTO skill_versions (
    workspace_id, skill_id, source_id, version_number,
    content_hash, package_object_key, manifest, license_expression
) VALUES (
    $1, $2, $3,
    (SELECT coalesce(max(version_number), 0) + 1 FROM skill_versions WHERE skill_id = $2),
    $4, $5, $6, $7
)
RETURNING *;

-- name: GetSkillVersion :one
SELECT * FROM skill_versions
WHERE id = $1 AND workspace_id = $2;

-- name: ListSkillVersions :many
SELECT * FROM skill_versions
WHERE workspace_id = $1 AND skill_id = $2
ORDER BY version_number DESC;
