ALTER TABLE run_attempts ADD COLUMN provider_unreachable_since timestamptz;

DROP TRIGGER run_attempts_immutable ON run_attempts;
CREATE TRIGGER run_attempts_immutable
    BEFORE UPDATE OR DELETE ON run_attempts
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable(
        'provider_run_id', 'error_class', 'error_message', 'started_at', 'finished_at',
        'object_grants_expire_at', 'object_grants_state', 'provider_unreachable_since');
