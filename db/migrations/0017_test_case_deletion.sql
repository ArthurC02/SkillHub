ALTER TABLE test_cases ADD COLUMN deleted_at timestamptz;

CREATE INDEX test_cases_workspace_live_idx ON test_cases (workspace_id, created_at DESC)
    WHERE deleted_at IS NULL;

ALTER TABLE test_cases ADD CONSTRAINT test_cases_name_not_blank CHECK (btrim(name) <> '');
