CREATE TABLE run_permission_confirmations (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id     uuid NOT NULL REFERENCES workspaces (id),
    skill_version_id uuid NOT NULL REFERENCES skill_versions (id),
    test_case_id     uuid NOT NULL REFERENCES test_cases (id),
    summary_hash     text NOT NULL,
    confirmed_by     uuid NOT NULL REFERENCES users (id),
    confirmed_at     timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX run_permission_confirmations_key
    ON run_permission_confirmations (workspace_id, skill_version_id, test_case_id, summary_hash);
