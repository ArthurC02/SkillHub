-- Soft-deleted artifact rows remain durable cleanup work until purged_at is
-- stamped. The original 0045 partial index excluded them, matching the old
-- worklist that could never retry a failed user-requested object deletion.

DROP INDEX artifacts_retention_fairness_idx;
CREATE INDEX artifacts_retention_fairness_idx
    ON artifacts (retention_attempted_at NULLS FIRST, expires_at, id)
    WHERE purged_at IS NULL;
