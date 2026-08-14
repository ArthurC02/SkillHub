-- name: GetLatestSkillVersion :one
SELECT * FROM skill_versions
WHERE skill_id = $1 AND workspace_id = $2
ORDER BY version_number DESC
LIMIT 1;
