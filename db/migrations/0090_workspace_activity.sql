ALTER TABLE runs
    ADD COLUMN activity_updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE runs DISABLE TRIGGER runs_terminal_immutable;

UPDATE runs
SET activity_updated_at = greatest(
    created_at,
    coalesce(started_at, created_at),
    coalesce(finished_at, created_at),
    coalesce(cancel_requested_at, created_at)
);

ALTER TABLE runs ENABLE TRIGGER runs_terminal_immutable;

CREATE INDEX runs_workspace_activity_idx
    ON runs (workspace_id, activity_updated_at DESC, id);
