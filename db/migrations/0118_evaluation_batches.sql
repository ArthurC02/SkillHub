CREATE TABLE evaluation_batches (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id     uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    skill_id         uuid NOT NULL,
    skill_version_id uuid NOT NULL,
    trials           integer NOT NULL CHECK (trials BETWEEN 1 AND 10),
    status           text NOT NULL DEFAULT 'launching' CHECK (status IN ('launching', 'launched', 'stopped')),
    stop_reason      text,
    created_by       uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT evaluation_batches_stop_has_reason CHECK ((status = 'stopped') = (stop_reason IS NOT NULL)),
    CONSTRAINT evaluation_batches_id_workspace_key UNIQUE (id, workspace_id),
    CONSTRAINT evaluation_batches_skill_fkey
        FOREIGN KEY (skill_id, workspace_id) REFERENCES skills (id, workspace_id) ON DELETE CASCADE,
    CONSTRAINT evaluation_batches_version_fkey
        FOREIGN KEY (skill_version_id, workspace_id) REFERENCES skill_versions (id, workspace_id) ON DELETE CASCADE
);

CREATE INDEX evaluation_batches_skill_idx ON evaluation_batches (workspace_id, skill_id, created_at DESC);
CREATE INDEX evaluation_batches_launching_idx ON evaluation_batches (workspace_id) WHERE status = 'launching';

COMMENT ON TABLE evaluation_batches IS
    'One evaluation set run against one skill version, every test case the given number of trials.';

CREATE TABLE evaluation_batch_items (
    batch_id     uuid NOT NULL,
    workspace_id uuid NOT NULL,
    test_case_id uuid NOT NULL,
    position     integer NOT NULL CHECK (position >= 0),
    trial        integer NOT NULL CHECK (trial >= 1),
    summary_hash text NOT NULL,
    run_id       uuid UNIQUE,
    PRIMARY KEY (batch_id, test_case_id, trial),
    CONSTRAINT evaluation_batch_items_batch_fkey
        FOREIGN KEY (batch_id, workspace_id) REFERENCES evaluation_batches (id, workspace_id) ON DELETE CASCADE,
    CONSTRAINT evaluation_batch_items_test_case_fkey
        FOREIGN KEY (test_case_id, workspace_id) REFERENCES test_cases (id, workspace_id) ON DELETE CASCADE,
    CONSTRAINT evaluation_batch_items_run_fkey
        FOREIGN KEY (run_id, workspace_id) REFERENCES runs (id, workspace_id) ON DELETE CASCADE
);

CREATE INDEX evaluation_batch_items_pending_idx ON evaluation_batch_items (batch_id, trial, position) WHERE run_id IS NULL;

COMMENT ON TABLE evaluation_batch_items IS
    'One trial of one test case in a batch; the run is linked once the batch starts it.';
