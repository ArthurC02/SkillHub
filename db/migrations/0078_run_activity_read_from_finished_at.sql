ALTER TABLE runs DISABLE TRIGGER runs_terminal_immutable;

UPDATE runs SET finished_at = coalesce(started_at, created_at)
WHERE status IN ('succeeded', 'failed', 'cancelled', 'timed_out') AND finished_at IS NULL;

UPDATE runs SET finished_at = NULL
WHERE status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out') AND finished_at IS NOT NULL;

ALTER TABLE runs ENABLE TRIGGER runs_terminal_immutable;

ALTER TABLE runs ADD CONSTRAINT runs_finished_exactly_when_terminal
    CHECK ((finished_at IS NOT NULL) = (status IN ('succeeded', 'failed', 'cancelled', 'timed_out')));

DROP INDEX runs_active_idx;
CREATE INDEX runs_active_idx ON runs (created_at) WHERE finished_at IS NULL;

DROP INDEX runs_supervision_fairness_idx;
CREATE INDEX runs_supervision_fairness_idx
    ON runs (supervision_checked_at NULLS FIRST, created_at, id)
    WHERE finished_at IS NULL;

DROP INDEX runs_cleanup_fairness_idx;
CREATE INDEX runs_cleanup_fairness_idx
    ON runs (cleanup_attempted_at NULLS FIRST, finished_at, id)
    WHERE finished_at IS NOT NULL AND cleanup_status <> 'cleaned';
