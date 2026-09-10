CREATE TABLE skill_sources (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    source_type  text NOT NULL CHECK (source_type IN ('git', 'upload')),
    source_url   text,
    source_ref   text,          -- commit SHA or tag; NULL for uploads
    content_hash text NOT NULL, -- hash of the fetched package as a whole
    fetched_at   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT skill_sources_git_needs_url CHECK (source_type <> 'git' OR source_url IS NOT NULL)
);

CREATE INDEX skill_sources_workspace_id_idx ON skill_sources (workspace_id);
CREATE INDEX skill_sources_content_hash_idx ON skill_sources (content_hash);

CREATE TABLE skills (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    name         text NOT NULL,
    summary      text,
    forked_from_skill_id   uuid REFERENCES skills (id),
    forked_from_version_id uuid,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX skills_workspace_id_idx ON skills (workspace_id);
CREATE UNIQUE INDEX skills_workspace_name_key ON skills (workspace_id, name);

CREATE TABLE skill_versions (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id       uuid NOT NULL REFERENCES workspaces (id),
    skill_id           uuid NOT NULL REFERENCES skills (id),
    source_id          uuid REFERENCES skill_sources (id),
    version_number     integer NOT NULL CHECK (version_number > 0),
    content_hash       text NOT NULL,
    package_object_key text NOT NULL, -- object storage holds the package itself (ADR-003)
    manifest           jsonb NOT NULL DEFAULT '{}'::jsonb, -- SKILL.md frontmatter as parsed
    license_expression text,          -- SPDX id as declared in the package; NULL = unknown
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX skill_versions_workspace_id_idx ON skill_versions (workspace_id);
CREATE UNIQUE INDEX skill_versions_number_key ON skill_versions (skill_id, version_number);
CREATE UNIQUE INDEX skill_versions_content_key ON skill_versions (skill_id, content_hash);

ALTER TABLE skills
    ADD CONSTRAINT skills_forked_from_version_id_fkey
    FOREIGN KEY (forked_from_version_id) REFERENCES skill_versions (id);
