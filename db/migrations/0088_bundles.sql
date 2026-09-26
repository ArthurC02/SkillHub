CREATE TABLE bundles (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    name         text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);

CREATE TABLE bundle_versions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    bundle_id    uuid NOT NULL REFERENCES bundles (id) ON DELETE CASCADE,
    version      text NOT NULL,
    description  text NOT NULL,
    content_hash text NOT NULL,
    created_by   uuid NOT NULL REFERENCES users (id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bundle_id, version)
);

CREATE TABLE bundle_members (
    bundle_version_id uuid NOT NULL REFERENCES bundle_versions (id) ON DELETE CASCADE,
    skill_id          uuid NOT NULL,
    skill_version_id  uuid NOT NULL REFERENCES skill_versions (id),
    version_number    integer NOT NULL,
    manifest_name     text NOT NULL,
    content_hash      text NOT NULL,
    position          integer NOT NULL,
    PRIMARY KEY (bundle_version_id, skill_id),
    UNIQUE (bundle_version_id, manifest_name),
    UNIQUE (bundle_version_id, position)
);

CREATE INDEX bundle_members_skill_version_idx ON bundle_members (skill_version_id);

CREATE TRIGGER bundle_versions_immutable
    BEFORE UPDATE OR DELETE ON bundle_versions
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

CREATE TRIGGER bundle_members_immutable
    BEFORE UPDATE OR DELETE ON bundle_members
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

ALTER TABLE publications
    ALTER COLUMN skill_id DROP NOT NULL,
    ADD COLUMN bundle_id uuid UNIQUE REFERENCES bundles (id) ON DELETE CASCADE,
    ADD CONSTRAINT publications_one_subject CHECK (num_nonnulls(skill_id, bundle_id) = 1);

ALTER TABLE publication_releases
    ALTER COLUMN skill_version_id DROP NOT NULL,
    ALTER COLUMN version_number DROP NOT NULL,
    ADD COLUMN bundle_version_id uuid REFERENCES bundle_versions (id) ON DELETE CASCADE,
    ADD CONSTRAINT publication_releases_one_subject
        CHECK (num_nonnulls(skill_version_id, bundle_version_id) = 1
               AND (skill_version_id IS NULL) = (version_number IS NULL));

ALTER TABLE download_artifacts
    ALTER COLUMN skill_version_id DROP NOT NULL,
    ADD COLUMN plugin_name text,
    ADD COLUMN plugin_version text,
    ADD CONSTRAINT download_artifacts_one_subject
        CHECK ((skill_version_id IS NULL) = (plugin_name IS NOT NULL)
               AND (plugin_name IS NULL) = (plugin_version IS NULL));

CREATE TABLE download_artifact_members (
    artifact_id      uuid NOT NULL,
    workspace_id     uuid NOT NULL,
    skill_version_id uuid NOT NULL REFERENCES skill_versions (id),
    position         integer NOT NULL,
    PRIMARY KEY (artifact_id, skill_version_id),
    CONSTRAINT download_artifact_members_artifact_fkey
        FOREIGN KEY (artifact_id, workspace_id)
        REFERENCES download_artifacts (artifact_id, workspace_id)
);

CREATE INDEX download_artifact_members_skill_version_idx ON download_artifact_members (skill_version_id);

CREATE TRIGGER download_artifact_members_immutable
    BEFORE UPDATE OR DELETE ON download_artifact_members
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();
