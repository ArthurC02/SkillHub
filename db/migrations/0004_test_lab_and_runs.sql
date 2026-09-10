CREATE TABLE test_cases (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        uuid NOT NULL REFERENCES workspaces (id),
    skill_id            uuid NOT NULL REFERENCES skills (id),
    name                text NOT NULL,
    user_prompt         text NOT NULL CHECK (btrim(user_prompt) <> ''), -- TEST-001
    acceptance_criteria jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX test_cases_workspace_id_idx ON test_cases (workspace_id);
CREATE INDEX test_cases_skill_id_idx ON test_cases (skill_id);

CREATE TABLE datasets (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    test_case_id uuid NOT NULL REFERENCES test_cases (id),
    file_name    text NOT NULL,
    content_type text NOT NULL,
    size_bytes   bigint NOT NULL CHECK (size_bytes >= 0),
    content_hash text NOT NULL,
    object_key   text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    deleted_at   timestamptz
);

CREATE INDEX datasets_workspace_id_idx ON datasets (workspace_id);
CREATE INDEX datasets_test_case_id_idx ON datasets (test_case_id);
CREATE INDEX datasets_expires_at_idx ON datasets (expires_at) WHERE deleted_at IS NULL;

CREATE TABLE test_case_snapshots (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        uuid NOT NULL REFERENCES workspaces (id),
    test_case_id        uuid NOT NULL REFERENCES test_cases (id),
    user_prompt         text NOT NULL,
    acceptance_criteria jsonb NOT NULL,
    dataset_refs        jsonb NOT NULL DEFAULT '[]'::jsonb,
    content_hash        text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX test_case_snapshots_workspace_id_idx ON test_case_snapshots (workspace_id);
CREATE INDEX test_case_snapshots_test_case_id_idx ON test_case_snapshots (test_case_id);

CREATE TYPE run_status AS ENUM (
    'queued',
    'provisioning',
    'preparing',
    'running',
    'evaluating',
    'succeeded',
    'failed',
    'cancelled',
    'timed_out'
);

CREATE TYPE run_cleanup_status AS ENUM (
    'pending',
    'cleaning_up',
    'cleaned',
    'failed'
);

CREATE TABLE runs (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id          uuid NOT NULL REFERENCES workspaces (id),
    skill_version_id      uuid NOT NULL REFERENCES skill_versions (id),
    test_case_snapshot_id uuid NOT NULL REFERENCES test_case_snapshots (id),
    status                run_status NOT NULL DEFAULT 'queued',
    status_reason         text,
    attempt               integer NOT NULL DEFAULT 1 CHECK (attempt > 0),
    provider              text NOT NULL,
    provider_run_id       text,
    runtime_snapshot      jsonb NOT NULL DEFAULT '{}'::jsonb,
    policy_snapshot       jsonb NOT NULL DEFAULT '{}'::jsonb,
    cleanup_status        run_cleanup_status NOT NULL DEFAULT 'pending',
    cleanup_at            timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    started_at            timestamptz,
    finished_at           timestamptz
);

CREATE INDEX runs_workspace_id_idx ON runs (workspace_id, created_at DESC);
CREATE INDEX runs_skill_version_id_idx ON runs (skill_version_id);
CREATE UNIQUE INDEX runs_provider_run_id_key ON runs (provider, provider_run_id)
    WHERE provider_run_id IS NOT NULL;

CREATE TABLE run_status_transitions (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id       uuid NOT NULL REFERENCES runs (id),
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    from_status  run_status,
    to_status    run_status NOT NULL,
    reason       text,
    occurred_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX run_status_transitions_run_id_idx ON run_status_transitions (run_id, occurred_at);
CREATE INDEX run_status_transitions_workspace_id_idx ON run_status_transitions (workspace_id);

CREATE TABLE trace_events (
    id                 uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id       uuid NOT NULL REFERENCES workspaces (id),
    run_id             uuid NOT NULL REFERENCES runs (id),
    seq                bigint NOT NULL, -- producer sequence, lets gaps be detected
    occurred_at        timestamptz NOT NULL,
    event_type         text NOT NULL,
    source             text NOT NULL,
    status             text,
    payload            jsonb NOT NULL DEFAULT '{}'::jsonb,
    payload_object_key text,
    PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

CREATE INDEX trace_events_run_id_idx ON trace_events (run_id, seq);
CREATE INDEX trace_events_workspace_id_idx ON trace_events (workspace_id);

CREATE TABLE trace_events_2026_08 PARTITION OF trace_events
    FOR VALUES FROM ('2026-08-01 00:00:00+00') TO ('2026-09-01 00:00:00+00');

CREATE TABLE evaluations (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id      uuid NOT NULL REFERENCES workspaces (id),
    run_id            uuid NOT NULL REFERENCES runs (id),
    overall           text NOT NULL CHECK (overall IN ('met', 'partially_met', 'not_met', 'undetermined')),
    summary           text,
    criterion_results jsonb NOT NULL DEFAULT '[]'::jsonb,
    judge_model       text,
    feedback_helpful  boolean,
    feedback_comment  text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX evaluations_workspace_id_idx ON evaluations (workspace_id);
CREATE UNIQUE INDEX evaluations_run_id_key ON evaluations (run_id);

CREATE TABLE artifacts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    run_id       uuid REFERENCES runs (id), -- NULL for packaging downloads (PACK-001)
    kind         text NOT NULL CHECK (kind IN ('run_output', 'download_package')),
    file_name    text NOT NULL,
    content_type text NOT NULL,
    size_bytes   bigint NOT NULL CHECK (size_bytes >= 0),
    content_hash text NOT NULL,
    object_key   text NOT NULL,
    scan_status  text NOT NULL DEFAULT 'quarantined'
                 CHECK (scan_status IN ('quarantined', 'available', 'rejected')),
    expires_at   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz,
    CONSTRAINT artifacts_run_output_needs_run CHECK (kind <> 'run_output' OR run_id IS NOT NULL)
);

CREATE INDEX artifacts_workspace_id_idx ON artifacts (workspace_id);
CREATE INDEX artifacts_run_id_idx ON artifacts (run_id);
CREATE INDEX artifacts_expires_at_idx ON artifacts (expires_at) WHERE deleted_at IS NULL;
