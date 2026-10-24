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
UPDATE feedback_reports
SET workspace_id = NULL, user_id = NULL, message = NULL, page_path = NULL, run_id = NULL
WHERE workspace_id = $1;

-- name: InsertFeedbackReport :exec
INSERT INTO feedback_reports (workspace_id, user_id, kind, message, page_path, run_id, build_id)
VALUES (@workspace_id, @user_id, @kind, sqlc.arg(message)::text, @page_path, @run_id, @build_id);

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

-- name: CountSearchToDownloadJourneys :many
WITH events AS (
    SELECT session_id, event_name, occurred_at, workspace_id
    FROM analytics_events
    WHERE occurred_at >= @since::timestamptz
      AND event_name IN (@search_event::text, @download_event::text)
), first_searches AS (
    SELECT session_id, min(occurred_at) AS searched_at
    FROM events
    WHERE event_name = @search_event::text
    GROUP BY session_id, (occurred_at AT TIME ZONE 'UTC')::date
), journeys AS (
    SELECT f.searched_at, min(d.occurred_at) AS downloaded_at
    FROM first_searches f
    LEFT JOIN events d
      ON d.session_id = f.session_id
     AND d.event_name = @download_event::text
     AND d.workspace_id IS NOT NULL
     AND d.occurred_at >= f.searched_at
     AND d.occurred_at < f.searched_at + @journey_window::interval
    GROUP BY f.session_id, f.searched_at
)
SELECT (searched_at AT TIME ZONE 'UTC')::date AS day,
       (GROUPING((searched_at AT TIME ZONE 'UTC')::date) = 1)::boolean AS whole_period,
       count(*)::bigint AS searched,
       count(downloaded_at)::bigint AS downloaded,
       count(*) FILTER (WHERE downloaded_at <= searched_at + @promise::interval)::bigint AS within_promise,
       coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM downloaded_at - searched_at))
           FILTER (WHERE downloaded_at IS NOT NULL), 0)::float8 AS median_seconds
FROM journeys
GROUP BY GROUPING SETS (((searched_at AT TIME ZONE 'UTC')::date), ())
ORDER BY 2, 1;

-- name: CountExpiredFeedbackReports :one
SELECT count(*)::bigint FROM feedback_reports WHERE created_at < $1;

-- name: CountAnalyticsEventsBefore :one
SELECT count(*)::bigint FROM analytics_events WHERE occurred_at < $1;
