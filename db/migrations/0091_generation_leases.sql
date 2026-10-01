CREATE TABLE generation_leases (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL
);
