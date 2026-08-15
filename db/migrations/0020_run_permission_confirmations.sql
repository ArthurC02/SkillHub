-- 0020_run_permission_confirmations: the user's explicit agreement to the pre-run
-- permission summary (02:TEST-005, 03:TEST-008/009). SEC-002 gate B blocks a Run
-- whose summary was never confirmed, or whose confirmation no longer matches.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- Why a row and not a column on runs: the agreement exists *before* the run does,
-- and has to be checkable at the moment the run is created. A column could only
-- record it after the fact, which is not a gate.

CREATE TABLE run_permission_confirmations (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id     uuid NOT NULL REFERENCES workspaces (id),
    skill_version_id uuid NOT NULL REFERENCES skill_versions (id),
    test_case_id     uuid NOT NULL REFERENCES test_cases (id),
    -- sha256 over the canonical summary body (internal/run/preflight.go). The hash
    -- *is* the scope of the agreement: another dataset, another provider, a
    -- different resource ceiling all produce a different hash, so an old row can
    -- never satisfy a changed request. That is 02:TEST-005「不得沿用不相符的舊確認」
    -- expressed as a lookup key, rather than as an invalidation sweep somebody has
    -- to remember to run.
    summary_hash     text NOT NULL,
    confirmed_by     uuid NOT NULL REFERENCES users (id),
    confirmed_at     timestamptz NOT NULL DEFAULT now()
);

-- One row per distinct agreement: re-confirming the same summary refreshes when it
-- happened instead of piling up rows. A refusal writes nothing at all — there is no
-- "denied" state to store, because the absence of a row is already the block.
CREATE UNIQUE INDEX run_permission_confirmations_key
    ON run_permission_confirmations (workspace_id, skill_version_id, test_case_id, summary_hash);
