-- name: CreateSkillSource :one
INSERT INTO skill_sources (
    workspace_id, source_type, source_url, source_ref, content_hash, fetched_at,
    task_description, generator_model, generator_prompt_version, generation_inputs,
    counts_toward_generate_quota,
    plugin_name, plugin_version, plugin_repository
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
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
  AND counts_toward_generate_quota
  AND fetched_at > @since;

-- name: OldestSourceCheck :one
SELECT min(coalesce(last_checked_at, created_at))::timestamptz FROM skill_sources
WHERE source_type = sqlc.arg(source_type)::text;

-- name: AcquireGenerationLease :one
INSERT INTO generation_leases (workspace_id, expires_at)
VALUES (@workspace_id, now() + @lease::interval)
ON CONFLICT (workspace_id) DO UPDATE SET expires_at = EXCLUDED.expires_at
WHERE generation_leases.expires_at <= now()
RETURNING workspace_id;

-- name: ReleaseGenerationLease :exec
DELETE FROM generation_leases WHERE workspace_id = @workspace_id;
