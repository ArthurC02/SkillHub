-- Record cleanup before an upload touches object storage. The intent is deleted
-- in the same transaction that publishes the dataset row; a crash or definite
-- rollback therefore leaves one durable, retryable object key behind.
CREATE TABLE dataset_object_cleanup_intents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    object_key text NOT NULL UNIQUE,
    not_before timestamptz NOT NULL DEFAULT now() + interval '1 hour',
    attempted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX dataset_object_cleanup_intents_due_idx
    ON dataset_object_cleanup_intents
    (attempted_at NULLS FIRST, not_before, id);
