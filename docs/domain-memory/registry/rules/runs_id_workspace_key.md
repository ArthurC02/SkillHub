---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0103_workspace_scoped_foreign_keys.sql#L4-4"
    excerpt_sha256: "sha256:a54869cf8b056257458a4ae2e2349ec082928cffa2e65298cedaa7d3e1cc94c6"
---
runs keeps unique runs_id_workspace_key: CONSTRAINT runs_id_workspace_key UNIQUE (id, workspace_id)

# Relations
- Contexts: [run](/contexts/run)
