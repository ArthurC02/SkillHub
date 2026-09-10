DROP INDEX artifacts_retention_fairness_idx;
CREATE INDEX artifacts_retention_fairness_idx
    ON artifacts (retention_attempted_at NULLS FIRST, expires_at, id)
    WHERE purged_at IS NULL;
