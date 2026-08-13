-- 0003_skill_registry: skills, import sources, immutable versions, fork lineage (CORE-002).
-- Owned by the Skill Registry & Versioning module (ADR-002).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

-- Where an imported package came from and what exactly was fetched (INGEST-004, DISC-003).
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
-- Duplicate content is *identified* here and *rejected* by skill_versions_content_key
-- below, so re-importing the same package from a second URL keeps both lineages (SKILL-001).
CREATE INDEX skill_sources_content_hash_idx ON skill_sources (content_hash);

CREATE TABLE skills (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    name         text NOT NULL,
    summary      text,
    -- Fork lineage (WS-001, DISC-003): a fork stays traceable to its origin after it
    -- diverges. forked_from_version_id gets its FK at the end of this file.
    forked_from_skill_id   uuid REFERENCES skills (id),
    forked_from_version_id uuid,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX skills_workspace_id_idx ON skills (workspace_id);
CREATE UNIQUE INDEX skills_workspace_name_key ON skills (workspace_id, name);

-- Immutable: a saved version is a content snapshot and is never rewritten in place
-- (ADR-003 "不可變快照", iron rule 4). Hence no updated_at; enforced by trigger in 0005.
-- License provenance is stored as found in the package; the reviewable license *status*
-- belongs to the Trust & Supply Chain module and must not mutate this row.
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
-- Re-saving identical content does not create a second version (SKILL-001, INGEST-005).
CREATE UNIQUE INDEX skill_versions_content_key ON skill_versions (skill_id, content_hash);

ALTER TABLE skills
    ADD CONSTRAINT skills_forked_from_version_id_fkey
    FOREIGN KEY (forked_from_version_id) REFERENCES skill_versions (id);
