-- 0016_run_orchestration: per-attempt provider mapping, cancellation intent and the
-- transactional outbox (RUN-003, RUN-004, ADR-004, ADR-008 / iron rule 9).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- River's own tables (river_job, river_leader, ...) are deliberately NOT here. River
-- versions its schema with its releases and ships rivermigrate to apply it; copying a
-- snapshot into a frozen migration file guarantees drift the first time the dependency
-- is upgraded. services/platform/internal/runqueue owns that call. The two systems do
-- not overlap: nothing in db/queries reads a river_ table, and River never reads these.

-- RUN-003 ---------------------------------------------------------------------
-- The run_id -> provider_run_id mapping, one row per attempt.
--
-- 0004 put `attempt` and `provider_run_id` on runs itself, which meant a second attempt
-- overwrote the first attempt's provider id: the mapping for the failed attempt was
-- gone, and with it the ability to reconcile a sandbox that outlived it. ADR-004 names
-- run_attempt_id as a first-class identifier, so it gets a first-class row.
CREATE TABLE run_attempts (
    -- The run_attempt_id of ADR-004. Permanent and platform-owned, like run_id.
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id         uuid NOT NULL REFERENCES runs (id),
    workspace_id   uuid NOT NULL REFERENCES workspaces (id),
    attempt_number integer NOT NULL CHECK (attempt_number > 0),
    provider       text NOT NULL,
    -- The provider's ephemeral handle (iron rule 10). Nullable: an attempt exists from
    -- the moment it is dispatched, and the provider only names it once it has provisioned.
    provider_run_id text,
    -- RunError.class from contracts/openapi/sandbox-provider.yaml. Kept per attempt so
    -- "provisioning kept failing" and "the agent failed once" are distinguishable
    -- without reading messages (ADR-004 失敗與重試, RUN-004).
    error_class    text,
    error_message  text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    started_at     timestamptz,
    finished_at    timestamptz
);

-- Allocation and the retry guarantee in one constraint: attempt N+1 is a new row or it
-- is nothing. Concurrent dispatch loses here and the caller re-reads.
CREATE UNIQUE INDEX run_attempts_run_number_key ON run_attempts (run_id, attempt_number);
CREATE INDEX run_attempts_run_id_idx ON run_attempts (run_id, attempt_number DESC);
CREATE INDEX run_attempts_workspace_id_idx ON run_attempts (workspace_id);
-- Moved off runs: a provider id is unique per provider, and now it belongs to the
-- attempt that owns it. The partial predicate keeps un-provisioned attempts out.
CREATE UNIQUE INDEX run_attempts_provider_run_id_key ON run_attempts (provider, provider_run_id)
    WHERE provider_run_id IS NOT NULL;

-- Identity of an attempt is frozen; its outcome is what gets filled in later. DELETE is
-- refused outright, which is what stops a retry from erasing the attempt it replaced.
CREATE TRIGGER run_attempts_immutable
    BEFORE UPDATE OR DELETE ON run_attempts
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable(
        'provider_run_id', 'error_class', 'error_message', 'started_at', 'finished_at');

-- runs no longer carries either field: one mapping, one place. Nothing read them yet
-- (there was no runs query file before this migration), so this drops no behaviour.
ALTER TABLE runs
    DROP COLUMN attempt,
    DROP COLUMN provider_run_id;

-- RUN-004 ---------------------------------------------------------------------
-- Cancellation intent, recorded separately from the resulting state. A user may cancel
-- a queued/provisioning/preparing/running/evaluating run; the workload actually stopping
-- is asynchronous, and until it does the run is still in its old state. Setting `status`
-- straight to cancelled would be lying about a sandbox that is still burning CPU.
--
-- The transition to cancelled, and propagation to the provider, is RUN-006.
ALTER TABLE runs ADD COLUMN cancel_requested_at timestamptz;

-- Which attempt a transition belongs to. Nullable: the queued state exists before any
-- attempt is dispatched, and a terminal transition may be decided by the control plane
-- with no live attempt at all.
ALTER TABLE run_status_transitions ADD COLUMN run_attempt_id uuid REFERENCES run_attempts (id);

-- ADR-008 / iron rule 9 -------------------------------------------------------
-- Transactional outbox. A domain state change and the event announcing it commit
-- together or neither happens; a separate publisher then moves committed rows onward.
--
-- No immutability trigger here, unlike audit_events: this is a transport buffer, not
-- history. The durable record of what happened is run_status_transitions and
-- audit_events, and a publisher that cannot delete a drained row would need a second
-- retention mechanism for nothing.
CREATE TABLE outbox_events (
    event_id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type     text NOT NULL,
    event_version  integer NOT NULL DEFAULT 1,
    occurred_at    timestamptz NOT NULL DEFAULT now(),
    -- run_id for the Run workflow (ADR-008: platform run_id is the correlation, never
    -- a provider id).
    correlation_id uuid NOT NULL,
    causation_id   uuid,
    workspace_id   uuid REFERENCES workspaces (id),
    aggregate_type text NOT NULL,
    aggregate_id   uuid NOT NULL,
    -- Identifiers and outcome. Never secrets, prompts or package content (iron rule 11).
    payload        jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- NULL until a publisher has handed the event on. Consumers dedupe on event_id,
    -- so at-least-once delivery is safe (ADR-008).
    published_at   timestamptz
);

-- The publisher's only scan: oldest unpublished first. Partial, so the index stays the
-- size of the backlog rather than the size of the history.
CREATE INDEX outbox_events_unpublished_idx ON outbox_events (occurred_at)
    WHERE published_at IS NULL;
CREATE INDEX outbox_events_aggregate_idx ON outbox_events (aggregate_type, aggregate_id, occurred_at);
