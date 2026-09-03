-- Durable worklists for object writes whose database manifest can be lost when
-- a process exits between object-store I/O and the publishing transaction.
CREATE TABLE download_object_cleanup_intents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    object_key text NOT NULL UNIQUE,
    not_before timestamptz NOT NULL DEFAULT now() + interval '1 hour',
    attempted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX download_object_cleanup_intents_due_idx
    ON download_object_cleanup_intents
    (attempted_at NULLS FIRST, not_before, id);

-- Cleanup guards recheck by object key while holding the writer's advisory
-- lock. Keep a batch from turning into one full-table scan per candidate.
CREATE INDEX artifacts_live_object_key_idx ON artifacts (object_key)
    WHERE deleted_at IS NULL AND purged_at IS NULL;
CREATE INDEX datasets_live_object_key_idx ON datasets (object_key)
    WHERE deleted_at IS NULL AND purged_at IS NULL;

CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON download_object_cleanup_intents
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

CREATE TABLE run_artifact_upload_intents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_attempt_id uuid NOT NULL UNIQUE REFERENCES run_attempts(id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    object_key text NOT NULL UNIQUE,
    not_before timestamptz NOT NULL,
    attempted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX run_artifact_upload_intents_due_idx
    ON run_artifact_upload_intents
    (attempted_at NULLS FIRST, not_before, id);

-- Cover attempts created or updated by an old worker between migrations 0050
-- and this trigger becoming active. Existing manifested archives are harmless:
-- the collector rechecks their live rows before removing anything.
INSERT INTO run_artifact_upload_intents
    (run_attempt_id, workspace_id, object_key, not_before)
SELECT id, workspace_id,
       'run-artifacts/' || run_id::text || '/' || id::text || '/artifacts.tar',
       object_grants_expire_at + interval '90 days'
FROM run_attempts;

CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON run_artifact_upload_intents
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

CREATE FUNCTION remember_run_artifact_upload_intent() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO run_artifact_upload_intents
        (run_attempt_id, workspace_id, object_key, not_before)
    VALUES (
        NEW.id,
        NEW.workspace_id,
        'run-artifacts/' || NEW.run_id::text || '/' || NEW.id::text || '/artifacts.tar',
        NEW.object_grants_expire_at + interval '90 days'
    )
    ON CONFLICT (run_attempt_id) DO UPDATE
    SET not_before = excluded.not_before, attempted_at = NULL;
    RETURN NEW;
END;
$$;

CREATE TRIGGER run_attempt_artifact_upload_intent
AFTER UPDATE OF object_grants_expire_at ON run_attempts
FOR EACH ROW EXECUTE FUNCTION remember_run_artifact_upload_intent();
