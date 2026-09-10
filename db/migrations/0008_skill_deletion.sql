ALTER TABLE skills ADD COLUMN deleted_at timestamptz;

DROP INDEX skills_workspace_name_key;
CREATE UNIQUE INDEX skills_workspace_name_key
    ON skills (workspace_id, name) WHERE deleted_at IS NULL;
