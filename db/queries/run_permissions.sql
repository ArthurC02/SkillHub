-- name: ConfirmRunPermissions :one
INSERT INTO run_permission_confirmations (
    workspace_id, skill_version_id, test_case_id, summary_hash, confirmed_by
) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (workspace_id, skill_version_id, test_case_id, summary_hash)
DO UPDATE SET confirmed_at = now()
RETURNING *;

-- name: GetRunPermissionConfirmation :one
SELECT * FROM run_permission_confirmations
WHERE workspace_id = $1 AND skill_version_id = $2 AND test_case_id = $3 AND summary_hash = $4;
