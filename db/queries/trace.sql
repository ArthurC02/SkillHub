-- Run Trace ingestion and reading (TRACE-002~008, contracts/events/trace-event.schema.json).
--
-- workspace_id is never taken from the wire: the ingestion handler resolves it from
-- run_id under the platform's own authority (iron rule 3), and every read below is
-- workspace scoped.

-- name: InsertTraceEvent :execrows
-- The idempotent write (TRACE-008). Delivery is at-least-once, so the producer's
-- event_id is the dedupe key: a redelivery updates nothing and returns 0 rows, which
-- is how the caller counts duplicates without a second query. Never DO UPDATE - the
-- 0005 trigger makes trace_events append-only, and a redelivery is not a correction.
INSERT INTO trace_events (
    event_id, workspace_id, run_id, attempt, seq, occurred_at,
    event_type, source, status, schema_version, masked, masked_fields, payload, late
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: ListTraceEvents :many
-- The whole trace of one run, in the cross-producer order of TRACE-001 §7:
-- (occurred_at, emitted_by, seq). Producer clocks skew, so occurred_at only merges
-- the streams; seq is the authority inside one of them.
SELECT * FROM trace_events
WHERE run_id = $1 AND workspace_id = $2
ORDER BY occurred_at, source, attempt, seq;

-- name: ListTraceEventsAfter :many
-- Incremental advanced-view read. ingest_seq is assigned by the database and is
-- therefore stable even when producer clocks skew or several streams reuse seq.
SELECT * FROM trace_events
WHERE run_id = @run_id AND workspace_id = @workspace_id
  AND ingest_seq > @after_ingest_seq
ORDER BY ingest_seq
LIMIT @page_limit;

-- name: ListEvaluationTraceEvents :many
-- The evaluator never needs the whole raw trace in memory. Keep a recent tail
-- plus bounded early activation/error evidence; large script-log payloads
-- outside this window are never transferred or decoded by the worker.
WITH tail AS (
    SELECT ingest_seq, occurred_at, source, attempt, seq FROM trace_events
    WHERE trace_events.run_id = @evaluation_run_id AND trace_events.workspace_id = @evaluation_workspace_id
    ORDER BY occurred_at DESC, source DESC, attempt DESC, seq DESC
    LIMIT 501
),
tail_kept AS (
    SELECT ingest_seq FROM tail
    ORDER BY occurred_at DESC, source DESC, attempt DESC, seq DESC
    LIMIT 500
),
activations AS (
    SELECT ingest_seq FROM trace_events
    WHERE trace_events.run_id = @evaluation_run_id AND trace_events.workspace_id = @evaluation_workspace_id
      AND event_type = 'skill_activation'
    ORDER BY occurred_at, source, attempt, seq
    LIMIT 100
),
errors AS (
    SELECT ingest_seq FROM trace_events
    WHERE trace_events.run_id = @evaluation_run_id AND trace_events.workspace_id = @evaluation_workspace_id
      AND event_type = 'error'
    ORDER BY occurred_at, source, attempt, seq
    LIMIT 100
),
selected AS (
    SELECT * FROM tail_kept
    UNION
    SELECT * FROM activations
    UNION
    SELECT * FROM errors
)
SELECT trace_events.*, (SELECT count(*) > 500 FROM tail) AS evaluation_truncated
FROM selected
JOIN trace_events USING (ingest_seq)
ORDER BY trace_events.occurred_at, trace_events.source, trace_events.attempt, trace_events.seq;

-- name: GetTraceStreamHealth :many
-- Exact stream health without materialising every event in the application.
-- Only the first 1,000 missing ordinals are returned; missing_count is exact.
WITH scoped AS (
    SELECT attempt, source, seq, late,
           lag(seq, 1, 0) OVER (PARTITION BY attempt, source ORDER BY seq) AS previous_seq
    FROM trace_events
    WHERE run_id = @run_id AND workspace_id = @workspace_id
),
streams AS (
    SELECT attempt, source, count(*)::bigint AS received,
           max(seq)::bigint AS highest_seq,
           (max(seq) - count(*))::bigint AS missing_count,
           count(*) FILTER (WHERE late)::bigint AS late_events
    FROM scoped
    GROUP BY attempt, source
)
SELECT s.attempt, s.source, s.received, s.highest_seq, s.missing_count, s.late_events,
       coalesce(ARRAY(
           SELECT candidate
           FROM scoped e
           CROSS JOIN LATERAL generate_series(e.previous_seq + 1, e.seq - 1) AS candidate
           WHERE e.attempt = s.attempt AND e.source = s.source
           ORDER BY candidate
           LIMIT 1000
       ), ARRAY[]::bigint[])::bigint[] AS missing_seq
FROM streams s
ORDER BY s.attempt, s.source;

-- name: GetTraceGeneralFold :one
-- Fold the general view in PostgreSQL so polling transfers one aggregate row,
-- not every raw payload. User-visible repeated lists are bounded and their exact
-- totals are returned so truncation is explicit rather than silent.
WITH events AS (
    SELECT event_type, occurred_at
    FROM trace_events
    WHERE trace_events.run_id = @fold_run_id AND trace_events.workspace_id = @fold_workspace_id
),
skill_rows AS (
    SELECT payload, occurred_at, source, attempt, seq FROM trace_events
    WHERE trace_events.run_id = @fold_run_id AND trace_events.workspace_id = @fold_workspace_id AND event_type = 'skill_activation'
    ORDER BY occurred_at, source, attempt, seq LIMIT 100
),
error_rows AS (
    SELECT payload, occurred_at, source, attempt, seq FROM trace_events
    WHERE trace_events.run_id = @fold_run_id AND trace_events.workspace_id = @fold_workspace_id AND event_type = 'error'
    ORDER BY occurred_at, source, attempt, seq LIMIT 100
),
tool_rows AS (
    SELECT payload->>'tool_name' AS tool_name,
           payload->>'outcome' AS outcome,
           occurred_at, source, attempt, seq,
           CASE WHEN (payload->>'duration_ms') ~ '^[0-9]{1,18}$'
                THEN (payload->>'duration_ms')::bigint ELSE 0 END AS duration_ms
    FROM trace_events
    WHERE trace_events.run_id = @fold_run_id AND trace_events.workspace_id = @fold_workspace_id AND event_type = 'tool_call'
),
last_output AS (
    SELECT payload->>'text' AS text FROM trace_events
    WHERE trace_events.run_id = @fold_run_id AND trace_events.workspace_id = @fold_workspace_id
      AND event_type = 'agent_output' AND payload->>'kind' = 'final'
    ORDER BY occurred_at DESC, source DESC, attempt DESC, seq DESC LIMIT 1
),
usage_rows AS (
    SELECT payload->>'scope' AS scope,
           payload->>'model' AS model,
           payload->>'input_tokens' AS input_tokens,
           payload->>'output_tokens' AS output_tokens,
           payload->>'cost_usd' AS cost_usd,
           payload->>'cost_source' AS cost_source,
           occurred_at, source, attempt, seq
    FROM trace_events
    WHERE trace_events.run_id = @fold_run_id AND trace_events.workspace_id = @fold_workspace_id AND event_type = 'usage'
),
last_usage AS (
    SELECT model, cost_source FROM usage_rows
    ORDER BY occurred_at DESC, source DESC, attempt DESC, seq DESC LIMIT 1
),
run_total AS (
    SELECT input_tokens, output_tokens, cost_usd FROM usage_rows WHERE scope = 'run_total'
    ORDER BY occurred_at DESC, source DESC, attempt DESC, seq DESC LIMIT 1
),
usage_sum AS (
    SELECT
      least(coalesce(sum(CASE WHEN input_tokens ~ '^[0-9]{1,18}$'
                              THEN input_tokens::bigint ELSE 0 END), 0),
            9223372036854775807)::bigint AS input_tokens,
      least(coalesce(sum(CASE WHEN output_tokens ~ '^[0-9]{1,18}$'
                              THEN output_tokens::bigint ELSE 0 END), 0),
            9223372036854775807)::bigint AS output_tokens,
      sum(CASE WHEN cost_usd ~ '^[0-9]{1,12}(\.[0-9]{1,12})?$'
               THEN cost_usd::numeric ELSE NULL END) AS cost_usd
    FROM usage_rows WHERE scope IS DISTINCT FROM 'run_total'
)
SELECT jsonb_build_object(
  'skills', (SELECT coalesce(jsonb_agg(jsonb_build_object(
      'name', coalesce(payload->>'skill_name', ''),
      'decision', coalesce(payload->>'decision', ''),
      'reason', coalesce(payload->>'reason', '')
  ) ORDER BY occurred_at, source, attempt, seq), '[]'::jsonb) FROM skill_rows),
  'skills_total', (SELECT count(*) FROM events WHERE event_type = 'skill_activation'),
  'resources_read', (SELECT count(*) FROM events WHERE event_type = 'resource_read'),
  'tool_calls', jsonb_build_object(
      'total', (SELECT count(*) FROM tool_rows),
      'succeeded', (SELECT count(*) FROM tool_rows WHERE outcome = 'succeeded'),
      'failed', (SELECT count(*) FROM tool_rows WHERE outcome IS DISTINCT FROM 'succeeded'),
      'total_duration_ms', (SELECT least(coalesce(sum(duration_ms), 0), 9223372036854775807) FROM tool_rows),
      'slowest_duration_ms', (SELECT coalesce(max(duration_ms), 0) FROM tool_rows),
      'slowest_tool', coalesce((SELECT tool_name FROM tool_rows
                               WHERE duration_ms > 0
                               ORDER BY duration_ms DESC, occurred_at, source, attempt, seq LIMIT 1), '')
  ),
  'errors', (SELECT coalesce(jsonb_agg(jsonb_build_object(
      'category', coalesce(payload->>'category', ''),
      'code', coalesce(payload->>'code', ''),
      'message', coalesce(payload->>'message', '')
  ) ORDER BY occurred_at, source, attempt, seq), '[]'::jsonb) FROM error_rows),
  'errors_total', (SELECT count(*) FROM events WHERE event_type = 'error'),
  -- 設計系統 §2.12: a run in flight needs a fact saying how long since anything
  -- moved, because a spinner that never stops looks the same whether the run is
  -- working or wedged. Null while no event has arrived yet, which the caller
  -- renders as a named state rather than as a blank or as "0 seconds ago".
  'last_event_at', (SELECT to_char(max(occurred_at) AT TIME ZONE 'UTC',
                                   'YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM events),
  'final_output', coalesce((SELECT text FROM last_output), ''),
  'usage', CASE WHEN EXISTS (SELECT 1 FROM usage_rows) THEN jsonb_build_object(
      'model', coalesce((SELECT model FROM last_usage), ''),
      'input_tokens', coalesce(
          (SELECT CASE WHEN input_tokens ~ '^[0-9]{1,18}$'
                       THEN input_tokens::bigint END FROM run_total),
          (SELECT input_tokens FROM usage_sum)),
      'output_tokens', coalesce(
          (SELECT CASE WHEN output_tokens ~ '^[0-9]{1,18}$'
                       THEN output_tokens::bigint END FROM run_total),
          (SELECT output_tokens FROM usage_sum)),
      'cost_usd', coalesce(
          (SELECT CASE WHEN cost_usd ~ '^[0-9]{1,12}(\.[0-9]{1,12})?$'
                       THEN cost_usd::numeric END FROM run_total),
          (SELECT cost_usd FROM usage_sum)),
      'cost_source', coalesce((SELECT cost_source FROM last_usage), '')
  ) ELSE NULL END
) AS folded;

-- name: LockTraceIngestRun :exec
-- Establish the global trace writer lock hierarchy before a control-plane
-- writer takes its per-stream lock. The insert trigger re-enters this lock.
SELECT pg_advisory_xact_lock(hashtextextended(
    'trace-ingest:' || CAST(@run_id AS uuid)::text, 0
));

-- name: NextTraceSeq :one
-- The next gapless ordinal for one (run_id, attempt, source) stream. Called inside
-- the transaction that writes the event, so two concurrent writers on the same
-- stream serialize at the database rather than both observing the same maximum.
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
-- Resolves the run a signed ingestion token names: the workspace to scope the write
-- to (iron rule 3 - the workspace is never taken from the wire), and enough state to
-- decide whether an arriving event is late (TRACE-008).
--
-- The second statement in the repository with no workspace_id parameter, for the same
-- reason as GetRunAttemptForReconcile: the caller has no workspace to offer. The id
-- comes from an HMAC-signed token the platform minted for this one run, not from a
-- user, and nothing user-visible is returned - only the scope the write is then
-- confined to.
SELECT id, workspace_id, status, finished_at FROM runs WHERE id = $1;

-- name: CountRunsNeedingCleanup :one
-- O11Y-003: the cleanup backlog as a single number, for the gauge the supervisor
-- publishes each sweep. Same predicate as ListRunsNeedingCleanup without the one
-- minute floor: a backlog that has not aged yet is still a backlog.
SELECT count(*) FROM runs
WHERE status IN ('succeeded', 'failed', 'cancelled', 'timed_out')
  AND cleanup_status <> 'cleaned';

-- name: CountTraceMaskingInWindow :one
-- 03:SEC-012 detection: the `TraceMaskingStopped` P1 criterion of 02:SEC-010,
-- asked of the table rather than of Prometheus, so the platform can act on it
-- without waiting for a person to read an alert.
--
-- Same evidence as infra/observability/alerts.yml's rule of that name — events were
-- stored AND not one field was redacted — because 0019 stores both halves: every row
-- is `masked` by CHECK, and masked_fields holds what the masker actually hit. An
-- empty array means "the masker ran and found nothing", which is why the test has to
-- be a sum over a window rather than a per-row emptiness check.
--
-- Two counts and not one, because the rule is `[1h]` **plus** `for: 1h` and both
-- halves are its threshold. Firing on the expression alone would mean halting the
-- fleet on any quiet hour that happened to carry nothing worth redacting: the rule's
-- premise (「正常流量下 tool_call 的 arguments 與 script_log 的 message 幾乎必然有
-- 東西被遮」) is a statement about volume, and at low volume it is simply not true.
-- `for: 1h` on a `[1h]` window is satisfied exactly when the rolling window stayed
-- non-empty and redaction-free for an hour, which is what asking for traffic on both
-- sides of @recent says in one pass and without keeping state between sweeps.
--
-- 0019 typed the column jsonb without constraining it to an array, and a producer
-- that redacted nothing stores JSON `null` there rather than `[]` (trace/service.go
-- marshals a nil slice). Both count as zero redactions, which is the only reading
-- available and also the conservative one: a row that cannot say what was redacted
-- is not evidence that anything was.
--
-- occurred_at and not an ingestion timestamp, because the table has none. It is
-- producer time, so a producer whose clock is behind is counted into an earlier
-- window; NFR-004 wants the gap inside 3 seconds and the windows are hours, so this
-- costs nothing the rule was relying on.
SELECT count(*) FILTER (WHERE occurred_at >= @recent)::bigint AS recent_events,
       count(*) FILTER (WHERE occurred_at <  @recent)::bigint AS earlier_events,
       coalesce(sum(CASE WHEN jsonb_typeof(masked_fields) = 'array'
                         THEN jsonb_array_length(masked_fields) ELSE 0 END), 0)::bigint AS masked_fields
FROM trace_events
WHERE occurred_at >= @since;
