CREATE TABLE run_attempts (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id         uuid NOT NULL REFERENCES runs (id),
    workspace_id   uuid NOT NULL REFERENCES workspaces (id),
    attempt_number integer NOT NULL CHECK (attempt_number > 0),
    provider       text NOT NULL,
    provider_run_id text,
    error_class    text,
    error_message  text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    started_at     timestamptz,
    finished_at    timestamptz
);

CREATE UNIQUE INDEX run_attempts_run_number_key ON run_attempts (run_id, attempt_number);
CREATE INDEX run_attempts_run_id_idx ON run_attempts (run_id, attempt_number DESC);
CREATE INDEX run_attempts_workspace_id_idx ON run_attempts (workspace_id);
CREATE UNIQUE INDEX run_attempts_provider_run_id_key ON run_attempts (provider, provider_run_id)
    WHERE provider_run_id IS NOT NULL;

CREATE TRIGGER run_attempts_immutable
    BEFORE UPDATE OR DELETE ON run_attempts
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable(
        'provider_run_id', 'error_class', 'error_message', 'started_at', 'finished_at');

ALTER TABLE runs
    DROP COLUMN attempt,
    DROP COLUMN provider_run_id;

ALTER TABLE runs ADD COLUMN cancel_requested_at timestamptz;

ALTER TABLE run_status_transitions ADD COLUMN run_attempt_id uuid REFERENCES run_attempts (id);

CREATE TABLE outbox_events (
    event_id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type     text NOT NULL,
    event_version  integer NOT NULL DEFAULT 1,
    occurred_at    timestamptz NOT NULL DEFAULT now(),
    correlation_id uuid NOT NULL,
    causation_id   uuid,
    workspace_id   uuid REFERENCES workspaces (id),
    aggregate_type text NOT NULL,
    aggregate_id   uuid NOT NULL,
    payload        jsonb NOT NULL DEFAULT '{}'::jsonb,
    published_at   timestamptz
);

CREATE INDEX outbox_events_unpublished_idx ON outbox_events (occurred_at)
    WHERE published_at IS NULL;
CREATE INDEX outbox_events_aggregate_idx ON outbox_events (aggregate_type, aggregate_id, occurred_at);
