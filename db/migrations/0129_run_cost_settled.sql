ALTER TABLE runs
    ADD COLUMN cost_settled_at timestamptz,
    ADD COLUMN spend_rechecked_at timestamptz;

CREATE INDEX runs_awaiting_spend_idx ON runs (finished_at)
    WHERE cleanup_status = 'cleaned' AND cost_settled_at IS NULL;

DROP TRIGGER runs_terminal_immutable ON runs;
CREATE TRIGGER runs_terminal_immutable
    BEFORE UPDATE OR DELETE ON runs
    FOR EACH ROW
    WHEN (OLD.status IN ('succeeded', 'failed', 'cancelled', 'timed_out'))
    EXECUTE FUNCTION enforce_immutable('cleanup_status', 'cleanup_at', 'cleanup_attempted_at',
                                       'cost_settled_at', 'spend_rechecked_at');
