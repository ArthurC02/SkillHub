-- A soft-deleted dataset is no longer readable, but its object may still need
-- deletion when the first object-store call fails. Keep that work durable and
-- distinguish "hidden" from "bytes confirmed gone".
ALTER TABLE datasets ADD COLUMN purged_at timestamptz;

DROP INDEX datasets_retention_fairness_idx;
CREATE INDEX datasets_retention_fairness_idx
    ON datasets (retention_attempted_at NULLS FIRST, expires_at, id)
    WHERE purged_at IS NULL;
