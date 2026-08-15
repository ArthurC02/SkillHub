-- 0018_run_scheduling: run-level failure classification and the two scans the
-- orchestrator's background jobs read (RUN-005~008, ADR-004 失敗與重試).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- `cleanup_status` is deliberately absent: 0004 already carries it, together with
-- `cleanup_at`, and 0005 keeps exactly those two columns writable after a run goes
-- terminal. RUN-007 needed a state field, not a new one.

-- RUN-006 ---------------------------------------------------------------------
-- Why the *run* ended badly, in the platform's own vocabulary.
--
-- Not the same thing as run_attempts.error_class, which is RunError.class straight
-- from contracts/openapi/sandbox-provider.yaml and is per attempt. Three failed
-- provisions followed by a success have three error classes and no run failure;
-- this column answers "what killed the run", which is the question the retry policy
-- and the funnel metrics ask.
--
-- The vocabulary is fixed because the retry rule is written against it: only
-- provider_error is the platform's to retry. workload_error is the skill failing at
-- its own job, and retrying that just burns tokens to reach the same answer.
ALTER TABLE runs ADD COLUMN failure_class text
    CHECK (failure_class IS NULL OR failure_class IN (
        'provider_error',       -- the provider could not carry the attempt
        'workload_error',       -- the workload ran and reported failure
        'timeout',              -- soft limit reported by the provider, or the platform watchdog
        'cancelled',            -- the user asked
        'capability_mismatch',  -- no configured provider can run this request
        'platform_error'        -- the control plane's own fault
    ));

-- RUN-008 ---------------------------------------------------------------------
-- The supervisor's scan: every run that is still supposed to be moving. Partial, so
-- the index is the size of the live set (bounded by provider slots) rather than the
-- size of all history. Deliberately not workspace scoped — this is the control
-- plane reconciling itself, not a user reading their own data.
CREATE INDEX runs_active_idx ON runs (created_at)
    WHERE status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out');

-- RUN-007 ---------------------------------------------------------------------
-- The cleanup backlog: terminal runs whose sandbox has not been confirmed released.
-- A cleanup job is scheduled in the same transaction as the terminal transition, so
-- this scan only ever picks up the leftovers of a job that exhausted its retries.
CREATE INDEX runs_cleanup_backlog_idx ON runs (finished_at)
    WHERE cleanup_status <> 'cleaned';
