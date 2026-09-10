-- name: CountQuotaRuns :one
SELECT
    count(DISTINCT r.id)::bigint AS used,
    min(t.occurred_at)::timestamptz AS oldest
FROM runs r
JOIN run_status_transitions t ON t.run_id = r.id AND t.to_status = 'preparing'
WHERE r.workspace_id = @workspace_id
  AND t.occurred_at > @since
  AND (r.failure_class IS NULL OR r.failure_class NOT IN (
          'provider_error', 'platform_error', 'capability_mismatch'));

-- name: GetWorkspaceCreatedAt :one
SELECT created_at FROM workspaces WHERE id = $1;

-- name: InsertAnalyticsEvent :exec
INSERT INTO analytics_events (
    event_name, session_id, workspace_id,
    query_length, query_language, result_count, has_results, filters_applied,
    skill_id,
    artifact_id, target
) VALUES (
    @event_name, @session_id, @workspace_id,
    @query_length, @query_language, @result_count, @has_results, @filters_applied,
    @skill_id,
    @artifact_id, @target
);

-- name: DetachWorkspaceAnalytics :execrows
UPDATE analytics_events SET workspace_id = NULL WHERE workspace_id = $1;

-- name: DetachWorkspaceFeedback :execrows
UPDATE feedback_reports SET workspace_id = NULL, user_id = NULL
WHERE workspace_id = $1;

-- name: InsertFeedbackReport :exec
INSERT INTO feedback_reports (workspace_id, user_id, kind, message, page_path, run_id, build_id)
VALUES (@workspace_id, @user_id, @kind, @message, @page_path, @run_id, @build_id);

-- name: RunInWorkspace :one
SELECT EXISTS (SELECT 1 FROM runs WHERE id = $1 AND workspace_id = $2);

-- name: GetIdentityProviderIDs :many
SELECT provider, provider_user_id FROM user_identities WHERE user_id = $1;
-- name: DeleteExpiredFeedbackReports :execrows
DELETE FROM feedback_reports WHERE created_at < $1;
