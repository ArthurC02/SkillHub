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

-- name: NextTraceSeq :one
-- The next gapless ordinal for one (run_id, attempt, source) stream. Called inside
-- the transaction that writes the event, so two concurrent writers on the same
-- stream cannot both take the same number - the unique index on event_id does not
-- protect seq, the transaction does. Only the orchestrator writes through this path;
-- sandbox and llm_service producers count in their own process.
SELECT (coalesce(max(seq), 0) + 1)::bigint FROM trace_events
WHERE run_id = $1 AND attempt = $2 AND source = $3;

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
