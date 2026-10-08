-- name: CountQuotaRuns :one
SELECT
    count(DISTINCT r.id)::bigint AS used,
    min(t.occurred_at)::timestamptz AS oldest
FROM run_status_transitions t
JOIN runs r ON r.id = t.run_id
WHERE t.workspace_id = @workspace_id AND t.to_status = @counted_from_status
  AND t.occurred_at > @since
  AND r.workspace_id = @workspace_id
  AND (r.failure_class IS NULL OR r.failure_class <> ALL(@exempt_failure_classes::text[]));

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
-- name: DeleteAnalyticsEventsBefore :execrows
DELETE FROM analytics_events WHERE occurred_at < $1;

-- name: DeleteExpiredFeedbackReports :execrows
DELETE FROM feedback_reports WHERE created_at < $1;

-- name: RunIDsInWorkspace :many
SELECT id FROM runs WHERE id = ANY(@run_ids::uuid[]) AND workspace_id = @workspace_id;

-- name: CountFunnelReachByDay :many
SELECT (occurred_at AT TIME ZONE 'UTC')::date AS day,
       event_name,
       count(DISTINCT session_id)::bigint AS sessions,
       count(DISTINCT workspace_id)::bigint AS workspaces
FROM analytics_events
WHERE occurred_at >= @since::timestamptz
  AND event_name = ANY(@event_names::text[])
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: CountExpiredFeedbackReports :one
SELECT count(*)::bigint FROM feedback_reports WHERE created_at < $1;

-- name: CountAnalyticsEventsBefore :one
SELECT count(*)::bigint FROM analytics_events WHERE occurred_at < $1;
