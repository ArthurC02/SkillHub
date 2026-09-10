-- name: InsertCostEvent :one
-- One row per billable model call (creation step, catalog search embedding,
-- index enrichment, evaluation judge/suggest). Never stores query text
-- (ADR-029). workspace_id/user_id are both nullable for an anonymous catalog
-- search.
INSERT INTO cost_events (
    kind, model, prompt_version, prompt_tokens, completion_tokens, usd_micros,
    cost_source, workspace_id, user_id, ref_type, ref_id, idempotency_key
) VALUES (
    sqlc.arg(kind), sqlc.arg(model), sqlc.arg(prompt_version), sqlc.arg(prompt_tokens),
    sqlc.arg(completion_tokens), sqlc.arg(usd_micros), sqlc.arg(cost_source),
    sqlc.arg(workspace_id), sqlc.arg(user_id), sqlc.arg(ref_type), sqlc.arg(ref_id),
    sqlc.arg(idempotency_key)
) RETURNING *;

-- name: AggregateCostEventsWindow :one
-- Percentiles for one kind over [window_start, window_end), the input to
-- InsertCostStatistics. Called both event-driven (session end) and by the
-- daily River job (05 "統計兩種時機") — same query, different window bounds
-- and schedule, decided by the caller.
SELECT
    count(*)::bigint AS sample_count,
    percentile_cont(0.5) WITHIN GROUP (ORDER BY usd_micros)::bigint AS p50_usd_micros,
    percentile_cont(0.9) WITHIN GROUP (ORDER BY usd_micros)::bigint AS p90_usd_micros,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY usd_micros)::bigint AS p95_usd_micros,
    max(usd_micros)::bigint AS max_usd_micros
FROM cost_events
WHERE kind = sqlc.arg(kind)
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
-- The window the before-session and per-step gates derive their threshold
-- from (05 "門檻由統計推導"). Caller falls back to a configured constant when
-- sample_count < 20.
SELECT * FROM cost_statistics
WHERE kind = $1
ORDER BY window_end DESC
LIMIT 1;

-- name: PurgeExpiredCostEvents :execrows
-- Retention sweep, same shape as PurgeExpiredCreditEntries: caller sets
-- `SET LOCAL skillhub.purge = 'on'` first so this DELETE passes
-- cost_events_immutable.
DELETE FROM cost_events WHERE created_at < $1;

-- name: PurgeUserCostEvents :execrows
-- Account deletion, not retention: everything this one user's paid calls left
-- behind, whatever its age. The retention sweep above is time-based and cannot
-- serve this - the adversarial review of 2026-09-08 found the store interface
-- claiming otherwise, which would have left a deleted account's spend on file.
-- Same purge guard: `SET LOCAL skillhub.purge = 'on'` before this DELETE.
DELETE FROM cost_events WHERE user_id = $1;

-- name: GetCostEventByIdempotencyKey :one
-- The replay half of InsertCostEvent's idempotency. The insert above is a
-- plain INSERT and the codebase catches 23505 rather than upserting, which
-- leaves the caller holding a duplicate-key error and no id -- and credit's
-- Charge needs that id to point the debit entry at the cost event it came
-- from. Without this query a retried settlement would write a debit whose
-- cost_event_id is empty, which is precisely the "扣了錢但答不出為什麼"
-- ADR-068 decision 3 forbids.
SELECT id FROM cost_events WHERE idempotency_key = $1;
