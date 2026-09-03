-- A run can lose every output file to its frozen artifact limits. Keep that
-- fact on the run rather than only on surviving manifest rows, because an empty
-- manifest has no row on which a truncation marker could live.
ALTER TABLE runs
    ADD COLUMN artifacts_truncated boolean NOT NULL DEFAULT false;
