ALTER TABLE datasets ADD COLUMN purged_at timestamptz;

DROP INDEX datasets_retention_fairness_idx;
CREATE INDEX datasets_retention_fairness_idx
    ON datasets (retention_attempted_at NULLS FIRST, expires_at, id)
    WHERE purged_at IS NULL;
