-- name: InsertEvaluationGrader :one
INSERT INTO evaluation_graders (workspace_id, name, created_by)
VALUES (sqlc.narg(workspace_id), @name, @created_by)
RETURNING *;

-- name: GetEvaluationGrader :one
SELECT * FROM evaluation_graders
WHERE id = @id AND workspace_id IS NOT DISTINCT FROM sqlc.narg(workspace_id)
FOR UPDATE;

-- name: InsertEvaluationGraderVersion :one
INSERT INTO evaluation_grader_versions (grader_id, version, instructions, reason, created_by)
SELECT g.id, coalesce(max(v.version), 0) + 1, @instructions, @reason, @created_by
FROM evaluation_graders g
LEFT JOIN evaluation_grader_versions v ON v.grader_id = g.id
WHERE g.id = @grader_id AND g.workspace_id IS NOT DISTINCT FROM sqlc.narg(workspace_id)
GROUP BY g.id
RETURNING *;

-- name: ListVisibleEvaluationGraders :many
SELECT g.id, g.workspace_id, g.name, v.id AS version_id, v.version, v.instructions, v.created_at AS version_created_at
FROM evaluation_graders g
JOIN LATERAL (
    SELECT * FROM evaluation_grader_versions WHERE grader_id = g.id ORDER BY version DESC LIMIT 1
) v ON true
WHERE g.workspace_id = @workspace_id OR g.workspace_id IS NULL
ORDER BY g.workspace_id NULLS FIRST, g.name;

-- name: ListPlatformEvaluationGraders :many
SELECT g.id, g.workspace_id, g.name, v.id AS version_id, v.version, v.instructions, v.created_at AS version_created_at
FROM evaluation_graders g
JOIN LATERAL (
    SELECT * FROM evaluation_grader_versions WHERE grader_id = g.id ORDER BY version DESC LIMIT 1
) v ON true
WHERE g.workspace_id IS NULL
ORDER BY g.name;

-- name: GetVisibleGraderVersions :many
SELECT v.id, v.version, v.instructions, g.name
FROM evaluation_grader_versions v
JOIN evaluation_graders g ON g.id = v.grader_id
WHERE v.id = ANY(@ids::uuid[]) AND (g.workspace_id = @workspace_id OR g.workspace_id IS NULL);

-- name: PurgeWorkspaceEvaluationGraders :execrows
DELETE FROM evaluation_graders WHERE workspace_id = @workspace_id;
