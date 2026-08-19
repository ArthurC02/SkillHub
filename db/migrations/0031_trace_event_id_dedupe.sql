-- TRACE-008 requires event_id alone to be the idempotency key. PostgreSQL
-- cannot put that unique constraint on a time-partitioned table, so a trigger
-- serializes equal IDs and checks the indexed parent before each insert.

LOCK TABLE trace_events IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM trace_events GROUP BY event_id HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'trace_events contains duplicate event_id values; resolve them before migration 0031';
    END IF;
END $$;

CREATE FUNCTION dedupe_trace_event_id() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended('trace-event:' || NEW.event_id::text, 0));
    IF EXISTS (SELECT 1 FROM trace_events WHERE event_id = NEW.event_id) THEN
        RETURN NULL;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trace_events_dedupe_event_id
    BEFORE INSERT ON trace_events
    FOR EACH ROW EXECUTE FUNCTION dedupe_trace_event_id();
