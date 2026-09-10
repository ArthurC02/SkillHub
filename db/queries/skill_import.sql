-- name: CreateSkillSource :one
INSERT INTO skill_sources (
    workspace_id, source_type, source_url, source_ref, content_hash, fetched_at,
    task_description, generator_model, generator_prompt_version, generation_inputs
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetSkillByName :one
SELECT * FROM skills
WHERE workspace_id = $1 AND name = $2 AND deleted_at IS NULL;

-- name: GetVersionBySkillAndHash :one
SELECT * FROM skill_versions
WHERE skill_id = $1 AND content_hash = $2 AND workspace_id = $3;

-- name: CountGeneratedSkills :one
SELECT
    count(*)::bigint AS used,
    min(fetched_at)::timestamptz AS oldest
FROM skill_sources
WHERE workspace_id = @workspace_id
  AND source_type = 'generated'
  AND NOT COALESCE(generation_inputs @> '{"interactive": true}', false)
  AND fetched_at > @since;
