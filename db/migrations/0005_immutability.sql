-- One shared function. TG_ARGV lists columns that may still change; everything
-- else in the row is frozen, and DELETE is always rejected. Attach with a WHEN
-- clause when the freeze is conditional.
CREATE FUNCTION enforce_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    old_row jsonb;
    new_row jsonb;
    mutable_col text;
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'row in %.% is immutable and cannot be deleted (ADR-003)',
            TG_TABLE_SCHEMA, TG_TABLE_NAME
            USING ERRCODE = 'restrict_violation';
    END IF;

    old_row := to_jsonb(OLD);
    new_row := to_jsonb(NEW);

    IF TG_NARGS > 0 THEN
        FOREACH mutable_col IN ARRAY TG_ARGV LOOP
            old_row := old_row - mutable_col;
            new_row := new_row - mutable_col;
        END LOOP;
    END IF;

    IF old_row IS DISTINCT FROM new_row THEN
        RAISE EXCEPTION 'row in %.% is immutable and cannot be updated (ADR-003)',
            TG_TABLE_SCHEMA, TG_TABLE_NAME
            USING ERRCODE = 'restrict_violation';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER skill_versions_immutable
    BEFORE UPDATE OR DELETE ON skill_versions
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

CREATE TRIGGER test_case_snapshots_immutable
    BEFORE UPDATE OR DELETE ON test_case_snapshots
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

CREATE TRIGGER run_status_transitions_immutable
    BEFORE UPDATE OR DELETE ON run_status_transitions
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

CREATE TRIGGER trace_events_immutable
    BEFORE UPDATE OR DELETE ON trace_events
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

CREATE TRIGGER runs_terminal_immutable
    BEFORE UPDATE OR DELETE ON runs
    FOR EACH ROW
    WHEN (OLD.status IN ('succeeded', 'failed', 'cancelled', 'timed_out'))
    EXECUTE FUNCTION enforce_immutable('cleanup_status', 'cleanup_at');
