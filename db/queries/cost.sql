-- name: InsertCostEvent :one
INSERT INTO cost_events (
    kind, model, prompt_version, prompt_tokens, completion_tokens, usd_micros,
    cost_source, workspace_id, user_id, ref_type, ref_id, idempotency_key
) VALUES (
    sqlc.arg(kind), sqlc.arg(model), sqlc.arg(prompt_version), sqlc.arg(prompt_tokens),
    sqlc.arg(completion_tokens), sqlc.arg(usd_micros), sqlc.arg(cost_source),
    sqlc.arg(workspace_id), sqlc.arg(user_id), sqlc.arg(ref_type), sqlc.arg(ref_id),
    sqlc.arg(idempotency_key)
)
ON CONFLICT ON CONSTRAINT cost_events_idempotency_key_key DO NOTHING
RETURNING *;

-- name: AggregateCostEventsWindow :one
SELECT
    count(*)::bigint AS sample_count,
    coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY usd_micros), 0)::bigint AS p50_usd_micros,
    coalesce(percentile_cont(0.9) WITHIN GROUP (ORDER BY usd_micros), 0)::bigint AS p90_usd_micros,
    coalesce(percentile_cont(0.95) WITHIN GROUP (ORDER BY usd_micros), 0)::bigint AS p95_usd_micros,
    coalesce(max(usd_micros), 0)::bigint AS max_usd_micros
FROM cost_events
WHERE kind = sqlc.arg(kind) AND cost_source = sqlc.arg(measured_source)
  AND created_at >= sqlc.arg(window_start) AND created_at < sqlc.arg(window_end);

-- name: InsertCostStatistics :one
INSERT INTO cost_statistics (
    kind, window_start, window_end, sample_count,
    p50_usd_micros, p90_usd_micros, p95_usd_micros, max_usd_micros
) VALUES (
    sqlc.arg(kind), sqlc.arg(window_start), sqlc.arg(window_end), sqlc.arg(sample_count),
    sqlc.arg(p50_usd_micros), sqlc.arg(p90_usd_micros), sqlc.arg(p95_usd_micros),
    sqlc.arg(max_usd_micros)
) RETURNING *;

-- name: GetLatestCostStatistics :one
SELECT * FROM cost_statistics
WHERE kind = $1
ORDER BY window_end DESC
LIMIT 1;

-- name: SumCostEventsByDay :many
SELECT (created_at AT TIME ZONE 'UTC')::date AS day, kind,
       count(*)::bigint AS events, sum(usd_micros)::bigint AS usd_micros
FROM cost_events
WHERE created_at >= @since::timestamptz
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: GetCostEventByIdempotencyKey :one
SELECT id FROM cost_events WHERE idempotency_key = $1;

-- name: ListSessionStepCosts :many
SELECT ref_id::uuid AS session_id, user_id, usd_micros, cost_source, created_at
FROM cost_events
WHERE kind = sqlc.arg(step_kind) AND ref_type = sqlc.arg(session_ref_type)::text
  AND ref_id = ANY(sqlc.arg(session_ids)::uuid[])
ORDER BY ref_id, created_at, id;

-- name: ListSessionsLastSteppedBetween :many
SELECT ref_id::uuid AS session_id
FROM cost_events
WHERE kind = sqlc.arg(step_kind) AND ref_type = sqlc.arg(session_ref_type)::text AND ref_id IS NOT NULL
GROUP BY ref_id
HAVING max(created_at) >= sqlc.arg(window_start) AND max(created_at) < sqlc.arg(idle_before);

-- name: UpsertSessionCostSummary :exec
INSERT INTO cost_session_summaries (session_id, user_id, usd_micros, steps, estimated, last_step_at)
VALUES (sqlc.arg(session_id), sqlc.narg(user_id), sqlc.arg(usd_micros), sqlc.arg(steps), sqlc.arg(estimated), sqlc.arg(last_step_at))
ON CONFLICT (session_id) DO UPDATE SET
    user_id = EXCLUDED.user_id, usd_micros = EXCLUDED.usd_micros, steps = EXCLUDED.steps,
    estimated = EXCLUDED.estimated, last_step_at = EXCLUDED.last_step_at;

-- name: AggregateSessionSummariesWindow :one
SELECT
    count(*)::bigint AS sample_count,
    coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY usd_micros), 0)::bigint AS p50_usd_micros,
    coalesce(percentile_cont(0.9) WITHIN GROUP (ORDER BY usd_micros), 0)::bigint AS p90_usd_micros,
    coalesce(percentile_cont(0.95) WITHIN GROUP (ORDER BY usd_micros), 0)::bigint AS p95_usd_micros,
    coalesce(max(usd_micros), 0)::bigint AS max_usd_micros
FROM cost_session_summaries
WHERE NOT estimated AND last_step_at >= sqlc.arg(window_start) AND last_step_at < sqlc.arg(window_end);

-- name: ListLatestCostStatistics :many
SELECT DISTINCT ON (kind) * FROM cost_statistics
ORDER BY kind, window_end DESC;
