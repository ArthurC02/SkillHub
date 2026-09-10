ALTER TABLE run_attempts
    ADD COLUMN object_grants_expire_at timestamptz NOT NULL
    DEFAULT 'infinity'::timestamptz,
    ADD COLUMN object_grants_state text NOT NULL DEFAULT 'legacy_unknown'
        CHECK (object_grants_state IN ('legacy_unknown', 'unissued', 'recorded', 'closed'));

DROP TRIGGER run_attempts_immutable ON run_attempts;
CREATE TRIGGER run_attempts_immutable
    BEFORE UPDATE OR DELETE ON run_attempts
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable(
        'provider_run_id', 'error_class', 'error_message', 'started_at', 'finished_at',
        'object_grants_expire_at', 'object_grants_state');

UPDATE run_attempts
SET object_grants_expire_at = now() + interval '20 minutes';
