-- Keep bounded background worklists fair when one item repeatedly fails.
-- The workers claim a page by stamping these bookkeeping columns before doing
-- external work, so a poison item remains retryable without monopolising every
-- subsequent page.

ALTER TABLE artifacts
    ADD COLUMN reconcile_checked_at timestamptz,
    ADD COLUMN retention_attempted_at timestamptz;

ALTER TABLE datasets
    ADD COLUMN reconcile_checked_at timestamptz,
    ADD COLUMN retention_attempted_at timestamptz;

ALTER TABLE search_documents
    ADD COLUMN enrichment_attempted_at timestamptz;

ALTER TABLE users
    ADD COLUMN purge_attempted_at timestamptz,
    ADD COLUMN purge_started_at timestamptz;

ALTER TABLE runs
    ADD COLUMN supervision_checked_at timestamptz,
    ADD COLUMN cleanup_attempted_at timestamptz;

-- Terminal runs are immutable except for cleanup bookkeeping. Extend the
-- original trigger's explicit allow-list rather than weakening the function.
DROP TRIGGER runs_terminal_immutable ON runs;
CREATE TRIGGER runs_terminal_immutable
    BEFORE UPDATE OR DELETE ON runs
    FOR EACH ROW
    WHEN (OLD.status IN ('succeeded', 'failed', 'cancelled', 'timed_out'))
    EXECUTE FUNCTION enforce_immutable('cleanup_status', 'cleanup_at', 'cleanup_attempted_at');

CREATE INDEX artifacts_reconcile_fairness_idx
    ON artifacts (reconcile_checked_at NULLS FIRST, created_at, id)
    WHERE kind = 'download_package' AND scan_status = 'available'
      AND deleted_at IS NULL AND purged_at IS NULL;

CREATE INDEX artifacts_retention_fairness_idx
    ON artifacts (retention_attempted_at NULLS FIRST, expires_at, id)
    WHERE deleted_at IS NULL AND purged_at IS NULL;

CREATE INDEX datasets_reconcile_fairness_idx
    ON datasets (reconcile_checked_at NULLS FIRST, created_at, id)
    WHERE deleted_at IS NULL;

CREATE INDEX datasets_retention_fairness_idx
    ON datasets (retention_attempted_at NULLS FIRST, expires_at, id)
    WHERE deleted_at IS NULL;

CREATE INDEX search_documents_enrichment_fairness_idx
    ON search_documents (enrichment_attempted_at NULLS FIRST, updated_at, skill_id)
    WHERE enrichment_status = 'pending';

CREATE INDEX users_purge_fairness_idx
    ON users (purge_attempted_at NULLS FIRST, deletion_requested_at, id)
    WHERE deletion_requested_at IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX runs_supervision_fairness_idx
    ON runs (supervision_checked_at NULLS FIRST, created_at, id)
    WHERE status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out');

CREATE INDEX runs_cleanup_fairness_idx
    ON runs (cleanup_attempted_at NULLS FIRST, finished_at, id)
    WHERE status IN ('succeeded', 'failed', 'cancelled', 'timed_out')
      AND cleanup_status <> 'cleaned';
