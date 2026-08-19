-- 0034_trace_incremental_reads: give readers a stable, server-owned watermark.
-- Producer sequence numbers are scoped per stream and producer timestamps can
-- arrive out of order, so neither is safe as an incremental HTTP cursor.

CREATE SEQUENCE trace_events_ingest_seq;

ALTER TABLE trace_events
    ADD COLUMN ingest_seq bigint NOT NULL DEFAULT nextval('trace_events_ingest_seq');

ALTER SEQUENCE trace_events_ingest_seq OWNED BY trace_events.ingest_seq;

-- nextval alone is allocation ordered, not commit ordered: transaction A can
-- reserve 10, transaction B commit 11, a reader advance to 11, then A commit 10.
-- Readers cursor per run, so serialize assignment per run and assign only after
-- that transaction owns the lock. The lock lives until commit.
ALTER TABLE trace_events ALTER COLUMN ingest_seq DROP DEFAULT;

CREATE FUNCTION assign_trace_ingest_seq() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    -- The row lock orders this insert against a concurrent terminal transition.
    -- It must be acquired before the advisory lock: run transitions already
    -- update this row before recording their trace event, and reversing that
    -- order would deadlock terminal transition against sandbox ingestion.
    SELECT status IN ('succeeded', 'failed', 'cancelled', 'timed_out')
      INTO NEW.late
      FROM runs
     WHERE id = NEW.run_id
     FOR SHARE;
    PERFORM pg_advisory_xact_lock(hashtextextended('trace-ingest:' || NEW.run_id::text, 0));
    NEW.ingest_seq := nextval('trace_events_ingest_seq');
    RETURN NEW;
END;
$$;

CREATE TRIGGER trace_events_assign_ingest_seq
    BEFORE INSERT ON trace_events
    FOR EACH ROW EXECUTE FUNCTION assign_trace_ingest_seq();

CREATE INDEX trace_events_run_ingest_seq_idx
    ON trace_events (run_id, workspace_id, ingest_seq);
