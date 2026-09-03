-- ROLLOUT: legacy_unknown is intentionally not auto-repaired. Drain old workers,
-- wait 21 minutes, then use the repair in the account-purge rollout runbook.
--
-- Pre-signed object grants cannot be revoked. Infinity is fail-closed for an
-- attempt created by an old worker during a rolling deploy; the current worker
-- replaces it with the exact deadline before minting any URL.
ALTER TABLE run_attempts
    ADD COLUMN object_grants_expire_at timestamptz NOT NULL
    DEFAULT 'infinity'::timestamptz,
    ADD COLUMN object_grants_state text NOT NULL DEFAULT 'legacy_unknown'
        CHECK (object_grants_state IN ('legacy_unknown', 'unissued', 'recorded', 'closed'));

-- The 0016 immutability trigger predates this column and would reject the
-- backfill below. Replace it before touching existing rows; the migration is a
-- single transaction, so application writers never observe the gap.
DROP TRIGGER run_attempts_immutable ON run_attempts;
CREATE TRIGGER run_attempts_immutable
    BEFORE UPDATE OR DELETE ON run_attempts
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable(
        'provider_run_id', 'error_class', 'error_message', 'started_at', 'finished_at',
        'object_grants_expire_at', 'object_grants_state');

-- Attempts which existed before this migration may have a grant with at most
-- the current 15-minute hard wall plus five-minute grant slack. Start the bound
-- at migration time so even a URL minted immediately before this transaction is
-- gone before purge proceeds. Attempts a still-running old binary creates after
-- this point remain infinity and therefore block rather than risk data loss.
UPDATE run_attempts
SET object_grants_expire_at = now() + interval '20 minutes';
