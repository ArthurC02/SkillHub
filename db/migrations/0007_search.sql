CREATE TABLE search_documents (
    skill_id     uuid PRIMARY KEY REFERENCES skills (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    name         text NOT NULL,
    summary      text NOT NULL DEFAULT '',
    tsv tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(summary, '')), 'B')
    ) STORED,
    embedding  vector(1536),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX search_documents_tsv_idx ON search_documents USING gin (tsv);
CREATE INDEX search_documents_workspace_id_idx ON search_documents (workspace_id);
