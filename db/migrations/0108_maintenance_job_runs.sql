CREATE TABLE maintenance_job_runs (
    job            text PRIMARY KEY,
    period_seconds integer NOT NULL CHECK (period_seconds > 0),
    registered_at  timestamptz NOT NULL DEFAULT now(),
    succeeded_at   timestamptz
);

GRANT SELECT, INSERT, UPDATE, DELETE ON maintenance_job_runs TO skillhub_purge;
