ALTER TABLE runs ADD COLUMN failure_class text
    CHECK (failure_class IS NULL OR failure_class IN (
        'provider_error',       -- the provider could not carry the attempt
        'workload_error',       -- the workload ran and reported failure
        'timeout',              -- soft limit reported by the provider, or the platform watchdog
        'cancelled',            -- the user asked
        'capability_mismatch',  -- no configured provider can run this request
        'platform_error'        -- the control plane's own fault
    ));

CREATE INDEX runs_active_idx ON runs (created_at)
    WHERE status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out');

CREATE INDEX runs_cleanup_backlog_idx ON runs (finished_at)
    WHERE cleanup_status <> 'cleaned';
