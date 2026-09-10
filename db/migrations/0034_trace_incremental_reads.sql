CREATE SEQUENCE trace_events_ingest_seq;

ALTER TABLE trace_events
    ADD COLUMN ingest_seq bigint NOT NULL DEFAULT nextval('trace_events_ingest_seq');

ALTER SEQUENCE trace_events_ingest_seq OWNED BY trace_events.ingest_seq;

-- nextval is allocation-ordered, not commit-ordered: a later transaction can commit
-- a higher number first. Assignment is serialized per run under the advisory lock
-- in the trigger below.
ALTER TABLE trace_events ALTER COLUMN ingest_seq DROP DEFAULT;

CREATE FUNCTION assign_trace_ingest_seq() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    -- Take this row lock before the advisory lock below: run transitions update this
    -- row before recording their trace event, so the reverse order would deadlock.
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
