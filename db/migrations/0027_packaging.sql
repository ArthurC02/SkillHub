ALTER TABLE skills
    ADD COLUMN redistribution text NOT NULL DEFAULT 'unknown'
        CHECK (redistribution IN ('allowed', 'blocked', 'unknown'));

COMMENT ON COLUMN skills.redistribution IS
    'May a Download Artifact be produced from this skill? Only ''allowed'' releases; ''unknown'' blocks. license_status = Confirmed must never set this on its own (CONTENT-002). Copied onto forks at fork time, like access_restriction. See 0027.';

CREATE UNIQUE INDEX artifacts_packaging_ref_key ON artifacts (id, kind, workspace_id);

CREATE TABLE download_artifacts (
    artifact_id uuid PRIMARY KEY,
    kind text NOT NULL DEFAULT 'download_package' CHECK (kind = 'download_package'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id),

    skill_version_id uuid NOT NULL REFERENCES skill_versions (id),

    target          text NOT NULL CHECK (btrim(target) <> ''),
    profile_version text NOT NULL CHECK (btrim(profile_version) <> ''),
    packager_version text NOT NULL CHECK (btrim(packager_version) <> ''),

    manifest_hash text NOT NULL CHECK (btrim(manifest_hash) <> ''),

    includes_test_cases boolean NOT NULL,

    CONSTRAINT download_artifacts_artifact_fkey
        FOREIGN KEY (artifact_id, kind, workspace_id)
        REFERENCES artifacts (id, kind, workspace_id)
);

CREATE UNIQUE INDEX download_artifacts_workspace_key
    ON download_artifacts (workspace_id, artifact_id);
CREATE INDEX download_artifacts_dedupe_idx
    ON download_artifacts (skill_version_id, target, packager_version, includes_test_cases);

CREATE TRIGGER download_artifacts_immutable
    BEFORE UPDATE OR DELETE ON download_artifacts
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

CREATE TABLE download_records (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    artifact_id  uuid NOT NULL,
    actor_user_id uuid NOT NULL REFERENCES users (id),
    downloaded_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT download_records_artifact_fkey
        FOREIGN KEY (artifact_id, workspace_id)
        REFERENCES download_artifacts (artifact_id, workspace_id)
);

CREATE INDEX download_records_workspace_id_idx
    ON download_records (workspace_id, downloaded_at DESC);
CREATE INDEX download_records_artifact_id_idx
    ON download_records (artifact_id, downloaded_at DESC);

CREATE TRIGGER download_records_immutable
    BEFORE UPDATE OR DELETE ON download_records
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();
