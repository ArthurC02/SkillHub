-- name: GetLatestSkillVersion :one
SELECT * FROM skill_versions
WHERE id = (
    SELECT line.id FROM skill_line_versions line
    WHERE line.skill_id = $1 AND line.workspace_id = $2
    ORDER BY line.version_number DESC
    LIMIT 1
);
