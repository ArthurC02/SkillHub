-- name: CreateSkillSource :one
INSERT INTO skill_sources (workspace_id, source_type, source_url, source_ref, content_hash, fetched_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetSkillByName :one
SELECT * FROM skills
WHERE workspace_id = $1 AND name = $2 AND deleted_at IS NULL;

-- name: GetVersionBySkillAndHash :one
-- Duplicate-content detection (SKILL-001, INGEST-005): same content on the
-- same skill returns the existing immutable version instead of a new row.
SELECT * FROM skill_versions
WHERE skill_id = $1 AND content_hash = $2;
