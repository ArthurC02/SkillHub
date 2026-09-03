-- Serves ListWorkspaceRuns when test_case_id narrows a workspace's history.
-- The snapshot index finds the immutable snapshots for the draft; this index
-- avoids scanning every Run to join those snapshots once a workspace grows.
CREATE INDEX runs_test_case_snapshot_id_idx ON runs (test_case_snapshot_id, created_at DESC);
