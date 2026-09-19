ALTER TABLE runs DROP CONSTRAINT runs_failure_class_check;

ALTER TABLE runs ADD CONSTRAINT runs_failure_class_check
    CHECK (failure_class IS NULL OR failure_class IN (
        'provider_error',       -- the provider could not carry the attempt
        'workload_error',       -- the workload ran and reported failure
        'timeout',              -- soft limit reported by the provider, or the platform watchdog
        'cancelled',            -- the user asked
        'capability_mismatch',  -- no configured provider can run this request
        'policy_refused',       -- the platform's own policy refused it before anything ran
        'platform_error'        -- the control plane's own fault
    ));
