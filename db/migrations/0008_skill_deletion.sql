-- 0008_skill_deletion: user deletion of private skills (WS-005, CORE-007).
-- Same pattern as users.deleted_at: soft delete now, hard purge after the
-- 30-day grace period (PDM-006 6.1) by a background job that gets its own
-- privileged path later. Version rows stay untouched, so the 0005
-- immutability trigger keeps holding absolutely, and fork lineage FKs stay
-- valid as historical facts.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

ALTER TABLE skills ADD COLUMN deleted_at timestamptz;

-- Allow re-creating a skill with the same name after deleting the old one.
DROP INDEX skills_workspace_name_key;
CREATE UNIQUE INDEX skills_workspace_name_key
    ON skills (workspace_id, name) WHERE deleted_at IS NULL;
