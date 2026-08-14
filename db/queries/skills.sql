-- Every read here is workspace scoped (iron rule 3). The caller resolves workspace_id
-- from the session, never from request input.

-- name: CreateSkill :one
INSERT INTO skills (workspace_id, name, summary, forked_from_skill_id, forked_from_version_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSkill :one
SELECT * FROM skills
WHERE id = $1 AND workspace_id = $2;

-- name: UpdateSkillSummary :exec
-- Mutable skill metadata only; versions themselves are immutable (iron rule 4).
UPDATE skills SET summary = $2, updated_at = now()
WHERE id = $1;

-- name: ListSkills :many
SELECT * FROM skills
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;
