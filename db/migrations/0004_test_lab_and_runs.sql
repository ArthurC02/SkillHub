-- 0004_test_lab_and_runs: test cases, datasets, runs, trace, evaluation, artifacts (CORE-003).
-- Spans the Test Lab, Run Orchestration and Evaluation modules (ADR-002).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

-- Editable draft. The frozen copy used by a run is test_case_snapshots (TEST-001).
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

-- Uploaded test data. Files live in object storage; only the reference is relational
-- (ADR-003). 90 day retention, user may delete earlier (PDM-006 6, TEST-002, NFR-002).
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
-- Retention sweep scans for expired rows rather than scheduling at creation time, so a
-- shortened retention policy applies to already-stored data (PDM-006 risk table).
CREATE INDEX datasets_expires_at_idx ON datasets (expires_at) WHERE deleted_at IS NULL;

-- Immutable snapshot taken when a run starts. Keeps the run traceable (though no longer
-- reproducible) after the user deletes the dataset files (ADR-003 "刪除與可追溯性").
-- Enforced by trigger in 0005.
CREATE TABLE test_case_snapshots (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        uuid NOT NULL REFERENCES workspaces (id),
    test_case_id        uuid NOT NULL REFERENCES test_cases (id),
    user_prompt         text NOT NULL,
    acceptance_criteria jsonb NOT NULL,
    -- [{dataset_id, file_name, content_hash}] - the hash outlives the file itself.
    dataset_refs        jsonb NOT NULL DEFAULT '[]'::jsonb,
    content_hash        text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX test_case_snapshots_workspace_id_idx ON test_case_snapshots (workspace_id);
CREATE INDEX test_case_snapshots_test_case_id_idx ON test_case_snapshots (test_case_id);

-- The standard run states of ADR-004 / RUN-002. `cleaning_up` is deliberately not in this
-- enum: ADR-004 places cleanup *after* a terminal state, so execution outcome and cleanup
-- outcome are recorded separately and cleanup lives in run_cleanup_status below.
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
    -- `id` is the permanent platform run_id (iron rule 10). provider_run_id is the
    -- provider's ephemeral id: never a key, never part of a permanent URL.
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id          uuid NOT NULL REFERENCES workspaces (id),
    skill_version_id      uuid NOT NULL REFERENCES skill_versions (id),
    test_case_snapshot_id uuid NOT NULL REFERENCES test_case_snapshots (id),
    status                run_status NOT NULL DEFAULT 'queued',
    status_reason         text,
    attempt               integer NOT NULL DEFAULT 1 CHECK (attempt > 0),
    provider              text NOT NULL,
    provider_run_id       text,
    -- ADR-003 immutable snapshot: agent/model and version, runtime profile, provider and
    -- capability snapshot. Provider-specific fields stay inside the JSON (NFR-006).
    runtime_snapshot      jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- Permission and network policy as confirmed before start (TEST-005, ADR-003).
    policy_snapshot       jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- Cleanup is tracked apart from the execution outcome and stays writable after the
    -- run reaches a terminal state (ADR-004, RUN-002).
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

-- Every state change records time and reason (RUN-002). Append only; kept permanently
-- because the funnel metrics read it (PDM-006 6).
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

-- Trace detail events (TRACE-001). Monthly range partitions so the 90 day retention of
-- PDM-006 6 is a DROP PARTITION rather than a bulk DELETE (ADR-014, ADR-018).
-- Large payloads go to object storage and only the key is stored here (ADR-014).
-- Secrets must be masked before insert (iron rule 11) - the DB cannot enforce that.
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

-- First partition. Creating the next month's partition ahead of time is an operational
-- job, not a migration.
CREATE TABLE trace_events_2026_08 PARTITION OF trace_events
    FOR VALUES FROM ('2026-08-01 00:00:00+00') TO ('2026-09-01 00:00:00+00');

CREATE TABLE evaluations (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id      uuid NOT NULL REFERENCES workspaces (id),
    run_id            uuid NOT NULL REFERENCES runs (id),
    -- EVAL-001 forbids an unexplained score, so the overall verdict is a fixed vocabulary.
    overall           text NOT NULL CHECK (overall IN ('met', 'partially_met', 'not_met', 'undetermined')),
    summary           text,
    -- One entry per acceptance criterion:
    -- {criterion_id, result: passed|failed|undetermined, source: rule|model|user, evidence}.
    -- `source` is what keeps a model judgement from being presented as fact (EVAL-001).
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
    -- Uploads land in quarantine and only become downloadable once checks pass (ADR-003).
    scan_status  text NOT NULL DEFAULT 'quarantined'
                 CHECK (scan_status IN ('quarantined', 'available', 'rejected')),
    -- Run output 30 days, download package 90 days (PDM-006 6). Absolute date so the UI
    -- can show it before upload and on the download page.
    expires_at   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz,
    CONSTRAINT artifacts_run_output_needs_run CHECK (kind <> 'run_output' OR run_id IS NOT NULL)
);

CREATE INDEX artifacts_workspace_id_idx ON artifacts (workspace_id);
CREATE INDEX artifacts_run_id_idx ON artifacts (run_id);
CREATE INDEX artifacts_expires_at_idx ON artifacts (expires_at) WHERE deleted_at IS NULL;
