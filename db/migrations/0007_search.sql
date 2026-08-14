-- 0007_search: search projection for skill discovery (INGEST-009, ADR-013).
-- Rebuildable projection, never a source of truth (ADR-010): reindex recreates
-- every row from skills + skill_versions.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

CREATE TABLE search_documents (
    skill_id     uuid PRIMARY KEY REFERENCES skills (id) ON DELETE CASCADE,
    -- Tenant boundary lives in the projection too (ADR-011): every query is
    -- workspace-scoped until curated public content introduces a public scope.
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    name         text NOT NULL,
    summary      text NOT NULL DEFAULT '',
    -- FTS leg of the hybrid retrieval (ADR-013). English config: skill names
    -- and descriptions are English-dominant; cross-language recall is the
    -- vector leg's job, not this column's.
    tsv tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(summary, '')), 'B')
    ) STORED,
    -- Vector leg (ADR-013, text-embedding-3-small per PDM-003); filled by the
    -- LLM enhancement pipeline when it lands. NULL = not yet embedded.
    embedding  vector(1536),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX search_documents_tsv_idx ON search_documents USING gin (tsv);
CREATE INDEX search_documents_workspace_id_idx ON search_documents (workspace_id);
