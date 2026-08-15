-- 0019_trace_ingestion: close the four gaps between trace-event.schema.json and the
-- trace_events table (TRACE-001 §8 缺口清單), so the collection pipeline of
-- TRACE-002~008 has somewhere to land.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- The table has had no writer since 0004 created it, so every column below can be
-- added NOT NULL without a backfill.

-- 缺口 1: event_id ---------------------------------------------------------------
-- The producer-assigned idempotency key. `id` is server-generated, so the same event
-- re-sent under at-least-once delivery would land as two rows with two ids. The
-- unique index carries the partition key, as a partitioned table requires, and the
-- writer uses ON CONFLICT DO NOTHING: a redelivery is a no-op, never an update.
ALTER TABLE trace_events ADD COLUMN event_id uuid NOT NULL;

CREATE UNIQUE INDEX trace_events_event_id_key ON trace_events (event_id, occurred_at);

-- 缺口 2: attempt -----------------------------------------------------------------
-- ADR-004 separates run_id from the attempt. Without this column a retry's events
-- share one timeline with the first attempt's, and `seq`'s (run_id, attempt,
-- emitted_by) scope cannot be expressed in the database at all.
ALTER TABLE trace_events ADD COLUMN attempt integer NOT NULL DEFAULT 1
    CHECK (attempt >= 1);

-- 缺口 3: schema_version ----------------------------------------------------------
-- Version governance (ADR-009 影響/成本與限制). Hidden inside the jsonb payload it
-- would cost a full scan to query across versions.
ALTER TABLE trace_events ADD COLUMN schema_version text NOT NULL DEFAULT '1.0';

-- 缺口 4: masking state -----------------------------------------------------------
-- Iron rule 11 / NFR-002: an unmasked event must not be persisted. The README left
-- the CHECK to the db owner's judgement; it is taken here, because "deliberately
-- keeping an unmasked event for incident investigation" is not a path this system
-- has - it would be the exact rule-11 violation the constraint exists to stop. The
-- masker runs in the ingestion handler, before the INSERT.
--
-- masked_fields holds JSON Pointers relative to `payload`. An empty array means the
-- masker ran and found nothing, which is not the same as never having run - and that
-- distinction is why `masked` is its own boolean rather than an emptiness test.
ALTER TABLE trace_events
    ADD COLUMN masked boolean NOT NULL DEFAULT false CHECK (masked),
    ADD COLUMN masked_fields jsonb NOT NULL DEFAULT '[]'::jsonb;

-- TRACE-008 ----------------------------------------------------------------------
-- An event that arrived after its run had already reached a terminal state. Accepted
-- rather than dropped (the sandbox may push its last batch after the platform has
-- given up on it), but flagged, so the advanced view can say "this arrived late"
-- instead of silently reordering a timeline the user already read.
ALTER TABLE trace_events ADD COLUMN late boolean NOT NULL DEFAULT false;

-- The stream read: everything for one run, grouped by the (attempt, source) streams
-- whose seq must be gapless. 0004's (run_id, seq) index stays for range scans but is
-- no longer distinctive, because seq is scoped per producer (see the README §8 note).
CREATE INDEX trace_events_stream_idx ON trace_events (run_id, attempt, source, seq);

-- Partitioning --------------------------------------------------------------------
-- 0004 created one month's partition and called further partitions an operational
-- job. There is no such job, so the first insert in September would fail and the
-- run's trace would be lost - the one failure mode the whole batch exists to prevent.
--
-- ponytail: a DEFAULT partition catches every date instead. The ceiling is that
-- attaching a real monthly partition later requires the default to hold no rows for
-- that month (detach, move, re-attach). Upgrade path when trace volume makes
-- retention-by-DROP-PARTITION matter (PDM-006): a scheduled job that pre-creates the
-- next month, plus a one-off drain of this default.
CREATE TABLE trace_events_default PARTITION OF trace_events DEFAULT;
