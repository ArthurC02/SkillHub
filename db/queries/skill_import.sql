-- name: CreateSkillSource :one
-- The three generator columns are NULL for git and upload; 0037's one-way CHECK
-- requires all three when source_type is 'generated' (GEN-005).
INSERT INTO skill_sources (
    workspace_id, source_type, source_url, source_ref, content_hash, fetched_at,
    task_description, generator_model, generator_prompt_version
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetSkillByName :one
SELECT * FROM skills
WHERE workspace_id = $1 AND name = $2 AND deleted_at IS NULL;

-- name: GetVersionBySkillAndHash :one
-- Duplicate-content detection (SKILL-001, INGEST-005): same content on the
-- same skill returns the existing immutable version instead of a new row.
SELECT * FROM skill_versions
WHERE skill_id = $1 AND content_hash = $2;
