-- Pre-run permission confirmations (02:TEST-005, SEC-002 gate B). Both statements
-- are workspace scoped; the caller resolves workspace_id from the session, never
-- from request input (iron rule 3).

-- name: ConfirmRunPermissions :one
-- Records the user's agreement to one exact summary. Idempotent by design: the same
-- summary confirmed twice is one agreement, re-dated.
INSERT INTO run_permission_confirmations (
    workspace_id, skill_version_id, test_case_id, summary_hash, confirmed_by
) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (workspace_id, skill_version_id, test_case_id, summary_hash)
DO UPDATE SET confirmed_at = now()
RETURNING *;

-- name: GetRunPermissionConfirmation :one
-- The gate B lookup: is there an agreement to *this* summary, in this workspace?
-- No row means unconfirmed or confirmed against a summary that has since changed —
-- the same answer either way, because both must block the run.
SELECT * FROM run_permission_confirmations
WHERE workspace_id = $1 AND skill_version_id = $2 AND test_case_id = $3 AND summary_hash = $4;
