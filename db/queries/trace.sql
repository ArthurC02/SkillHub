-- name: InsertTraceEvent :execrows
INSERT INTO trace_events (
    event_id, workspace_id, run_id, attempt, seq, occurred_at,
    event_type, source, status, schema_version, masked, masked_fields, payload
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: InsertTraceEvents :many
INSERT INTO trace_events (
    event_id, workspace_id, run_id, attempt, seq, occurred_at,
    event_type, source, status, schema_version, masked, masked_fields, payload
)
SELECT e.event_id, e.workspace_id, e.run_id, e.attempt, e.seq, e.occurred_at,
       e.event_type, e.source, nullif(e.status, ''), e.schema_version, e.masked, e.masked_fields, e.payload
FROM (
    SELECT generate_series(1, cardinality(@event_ids::uuid[])) AS position,
           unnest(@event_ids::uuid[]) AS event_id,
           unnest(@workspace_ids::uuid[]) AS workspace_id,
           unnest(@run_ids::uuid[]) AS run_id,
           unnest(@attempts::int[]) AS attempt,
           unnest(@seqs::bigint[]) AS seq,
           unnest(@occurred_ats::timestamptz[]) AS occurred_at,
           unnest(@event_types::text[]) AS event_type,
           unnest(@sources::text[]) AS source,
           unnest(@statuses::text[]) AS status,
           unnest(@schema_versions::text[]) AS schema_version,
           unnest(@masked::bool[]) AS masked,
           unnest(@masked_fields::jsonb[]) AS masked_fields,
           unnest(@payloads::jsonb[]) AS payload
) e
ORDER BY e.position
RETURNING event_id;

-- name: ListTraceEventsAfter :many
SELECT * FROM trace_events
WHERE run_id = @run_id AND workspace_id = @workspace_id
  AND ingest_seq > @after_ingest_seq
ORDER BY ingest_seq
LIMIT @page_limit;

-- name: ListEvaluationTraceEvents :many
-- Returns a recent tail plus bounded early activation and error evidence, never the whole trace.
WITH tail AS (
    SELECT ingest_seq, occurred_at, source, attempt, seq FROM trace_events
    WHERE trace_events.run_id = @evaluation_run_id AND trace_events.workspace_id = @evaluation_workspace_id
    ORDER BY occurred_at DESC, source DESC, attempt DESC, seq DESC
    LIMIT sqlc.arg(tail_events)::int + 1
),
tail_kept AS (
    SELECT ingest_seq FROM tail
    ORDER BY occurred_at DESC, source DESC, attempt DESC, seq DESC
    LIMIT sqlc.arg(tail_events)::int
),
activations AS (
    SELECT ingest_seq FROM trace_events
    WHERE trace_events.run_id = @evaluation_run_id AND trace_events.workspace_id = @evaluation_workspace_id
      AND event_type = @activation_event_type::text
    ORDER BY occurred_at, source, attempt, seq
    LIMIT @activation_events::int
),
errors AS (
    SELECT ingest_seq FROM trace_events
    WHERE trace_events.run_id = @evaluation_run_id AND trace_events.workspace_id = @evaluation_workspace_id
      AND event_type = @error_event_type::text
    ORDER BY occurred_at, source, attempt, seq
    LIMIT @error_events::int
),
selected AS (
    SELECT * FROM tail_kept
    UNION
    SELECT * FROM activations
    UNION
    SELECT * FROM errors
)
SELECT trace_events.*, (SELECT count(*) > sqlc.arg(tail_events)::int FROM tail) AS evaluation_truncated
FROM selected
JOIN trace_events ON trace_events.ingest_seq = selected.ingest_seq
    AND trace_events.run_id = @evaluation_run_id AND trace_events.workspace_id = @evaluation_workspace_id
ORDER BY trace_events.occurred_at, trace_events.source, trace_events.attempt, trace_events.seq;

-- name: GetTraceStreamHealth :many
-- Computes stream health in the database: only the first missing ordinals up to the
-- reported cap come back, while missing_count stays exact.
WITH streams AS (
    SELECT attempt, source, count(*)::bigint AS received,
           max(seq)::bigint AS highest_seq,
           (max(seq) - count(*))::bigint AS missing_count,
           count(*) FILTER (WHERE late)::bigint AS late_events,
           max(occurred_at)::timestamptz AS last_event_at
    FROM trace_events
    WHERE run_id = @run_id AND workspace_id = @workspace_id
    GROUP BY attempt, source
)
SELECT s.attempt, s.source, s.received, s.highest_seq, s.missing_count, s.late_events, s.last_event_at,
       coalesce(ARRAY(
           SELECT candidate
           FROM (
               SELECT e.seq, lag(e.seq, 1, 0) OVER (ORDER BY e.seq) AS previous_seq
               FROM trace_events e
               WHERE e.run_id = @run_id AND e.workspace_id = @workspace_id
                 AND e.attempt = s.attempt AND e.source = s.source
           ) stream
           CROSS JOIN LATERAL generate_series(stream.previous_seq + 1, stream.seq - 1) AS candidate
           WHERE s.missing_count > 0
           ORDER BY candidate
           LIMIT @missing_seq_reported::int
       ), ARRAY[]::bigint[])::bigint[] AS missing_seq
FROM streams s
ORDER BY s.attempt, s.source;

-- name: ListTraceGeneralFacts :many
SELECT e.event_type, e.source, e.attempt, e.seq,
       COALESCE(p.skill_name, '')::text AS skill_name, COALESCE(p.decision, '')::text AS decision, COALESCE(p.reason, '')::text AS reason,
       COALESCE(p.category, '')::text AS category, COALESCE(p.code, '')::text AS code, COALESCE(p.message, '')::text AS message,
       COALESCE(p.tool_name, '')::text AS tool_name, COALESCE(p.outcome, '')::text AS outcome, COALESCE(p.duration_ms, '')::text AS duration_ms,
       COALESCE(p.kind, '')::text AS kind, COALESCE(p.scope, '')::text AS scope, COALESCE(p.model, '')::text AS model,
       COALESCE(p.input_tokens, '')::text AS input_tokens, COALESCE(p.output_tokens, '')::text AS output_tokens,
       COALESCE(p.cost_usd, '')::text AS cost_usd, COALESCE(p.cost_source, '')::text AS cost_source
FROM trace_events e
CROSS JOIN LATERAL jsonb_to_record(COALESCE(jsonb_path_query_first(e.payload, 'strict $ ? (@.type() == "object")'), '{}')) AS p(
       skill_name text, decision text, reason text, category text, code text, message text, tool_name text, outcome text,
       duration_ms text, kind text, scope text, model text, input_tokens text, output_tokens text, cost_usd text, cost_source text)
WHERE e.run_id = @run_id AND e.workspace_id = @workspace_id AND e.event_type = ANY(@event_types::text[])
ORDER BY e.occurred_at, e.source, e.attempt, e.seq;

-- name: GetTraceEventText :one
SELECT COALESCE(payload->>'text', '')::text AS text
FROM trace_events
WHERE run_id = @run_id AND workspace_id = @workspace_id
  AND source = @source AND attempt = @attempt AND seq = @seq;

-- name: LockTraceIngestRun :exec
-- Takes the global trace-writer lock before any per-stream lock; the insert trigger
-- re-enters it.
SELECT pg_advisory_xact_lock(hashtextextended(
    'trace-ingest:' || CAST(@run_id AS uuid)::text, 0
));

-- name: NextTraceSeq :one
-- Runs inside the event-writing transaction, so concurrent writers on one stream
-- serialize here instead of reading the same maximum.
WITH stream_lock AS (
    SELECT pg_advisory_xact_lock(hashtextextended(
        'trace-stream:' || CAST(@run_id AS uuid)::text || ':' || CAST(@attempt AS integer)::text || ':' || CAST(@source AS text),
        0
    ))
)
SELECT (coalesce(max(seq), 0) + 1)::bigint
FROM trace_events, stream_lock
WHERE run_id = CAST(@run_id AS uuid)
  AND attempt = CAST(@attempt AS integer)
  AND source = CAST(@source AS text);

-- name: GetRunForTraceIngest :one
SELECT id, workspace_id, status, finished_at FROM runs WHERE id = $1;

-- name: CountRunsNeedingCleanup :one
SELECT count(*) FROM runs
WHERE finished_at IS NOT NULL
  AND cleanup_status <> 'cleaned';

-- name: CountTraceMaskingInWindow :one
SELECT count(*) FILTER (WHERE occurred_at >= @recent)::bigint AS recent_events,
       count(*) FILTER (WHERE occurred_at <  @recent)::bigint AS earlier_events,
       coalesce(sum(jsonb_array_length(masked_fields)) FILTER (WHERE jsonb_typeof(masked_fields) = 'array'), 0)::bigint AS masked_fields
FROM trace_events
WHERE occurred_at >= @since AND source = @source;

-- name: DeleteTraceEventsBefore :execrows
DELETE FROM trace_events WHERE occurred_at < $1;
