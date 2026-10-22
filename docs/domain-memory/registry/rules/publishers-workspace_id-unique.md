---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0087_publishing.sql#L3-3"
    excerpt_sha256: "sha256:959f3eb14ab44bfaeb903119a401c68c06ccb6bbaadec78e8741f3571959832d"
---
publishers keeps unique publishers.workspace_id: workspace_id uuid NOT NULL UNIQUE REFERENCES workspaces (id)

# Relations
- Contexts: [publishing](/contexts/publishing)
