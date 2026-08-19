-- Keep lifecycle invariants at the persistence boundary. Go still rejects bad
-- requests early; these guards cover maintenance SQL and concurrent producers.

CREATE FUNCTION enforce_run_status_transition() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status IS NOT DISTINCT FROM NEW.status THEN
        RETURN NEW;
    END IF;
    IF NOT (CASE OLD.status
        WHEN 'queued' THEN NEW.status IN ('provisioning', 'failed', 'cancelled', 'timed_out')
        WHEN 'provisioning' THEN NEW.status IN ('preparing', 'failed', 'cancelled', 'timed_out')
        WHEN 'preparing' THEN NEW.status IN ('running', 'failed', 'cancelled', 'timed_out')
        WHEN 'running' THEN NEW.status IN ('evaluating', 'failed', 'cancelled', 'timed_out')
        WHEN 'evaluating' THEN NEW.status IN ('succeeded', 'failed', 'cancelled', 'timed_out')
        ELSE false
    END) THEN
        RAISE EXCEPTION 'illegal run status transition: % -> %', OLD.status, NEW.status
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

-- Name sorts after runs_terminal_immutable so terminal-row writes keep the
-- established restrict_violation contract instead of being reclassified.
CREATE TRIGGER runs_validate_status_transition
BEFORE UPDATE OF status ON runs
FOR EACH ROW EXECUTE FUNCTION enforce_run_status_transition();

-- trace_events is partitioned by time, so PostgreSQL cannot enforce this global
-- logical key with a native unique constraint that omits occurred_at.
LOCK TABLE trace_events IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM trace_events
        GROUP BY run_id, attempt, source, seq
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'trace_events contains duplicate logical stream sequence values; resolve them before migration 0032';
    END IF;
END $$;

CREATE FUNCTION enforce_trace_stream_seq() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(
        'trace-stream:' || NEW.run_id::text || ':' || NEW.attempt::text || ':' || NEW.source,
        0
    ));
    IF EXISTS (
        SELECT 1 FROM trace_events
        WHERE run_id = NEW.run_id AND attempt = NEW.attempt
          AND source = NEW.source AND seq = NEW.seq
    ) THEN
        RAISE EXCEPTION 'trace stream sequence already exists'
            USING ERRCODE = 'unique_violation';
    END IF;
    RETURN NEW;
END;
$$;

-- Exact event-id redeliveries are consumed first by the alphabetically earlier
-- trace_events_dedupe_event_id trigger; this one reports conflicting event IDs.
CREATE TRIGGER trace_events_guard_stream_seq
BEFORE INSERT ON trace_events
FOR EACH ROW EXECUTE FUNCTION enforce_trace_stream_seq();
