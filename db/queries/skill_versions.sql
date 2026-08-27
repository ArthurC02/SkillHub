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

-- name: GetLatestVersionLicense :one
-- The licence claim recorded on a skill's newest version: the expression and the
-- tier it was read from, never one without the other (ADR-021 決策 1 — frontmatter
-- `MIT` and a repo-root `MIT` are not the same assertion, and flattening them into
-- one string is what ADR-021 §5.3's false positive was made of).
--
-- Read by the redistribution route so that releasing a skill has to name the
-- evidence it relied on and be contradicted when that evidence is not what the
-- snapshot records (05 R-3b). Newest version rather than a named one: the verdict
-- is on the skill, and the bytes a download would hand over come from its newest
-- version.
--
-- No workspace scope, for the same reason the route itself is cross-workspace: a
-- redistribution verdict is about a source, so it has to reach the catalogue entry
-- and every fork alike. What comes back is an SPDX expression and which file it was
-- read from — nothing workspace-private.
SELECT license_expression, license_source
FROM skill_versions
WHERE skill_id = $1
ORDER BY version_number DESC
LIMIT 1;
