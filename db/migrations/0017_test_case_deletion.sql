-- 0017_test_case_deletion: soft delete for test cases (TEST-001, WS-002, CORE-007).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

-- Hard delete is not available: test_case_snapshots reference test_cases and are
-- frozen by the 0005 trigger (ADR-003), so a run's provenance would have to be
-- destroyed to remove a draft. Same shape as skills.deleted_at (0008): the row
-- leaves every read now, the purge sweeps it after the grace period.
ALTER TABLE test_cases ADD COLUMN deleted_at timestamptz;

-- Reads are workspace scoped and skip deleted rows (iron rule 3).
CREATE INDEX test_cases_workspace_live_idx ON test_cases (workspace_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- 0004 already rejects a blank user_prompt (TEST-001 "非空白 User Prompt"). A test
-- case nobody can identify in a list is the same defect, so name gets the same
-- guard rather than relying on every future writer to trim first.
ALTER TABLE test_cases ADD CONSTRAINT test_cases_name_not_blank CHECK (btrim(name) <> '');
