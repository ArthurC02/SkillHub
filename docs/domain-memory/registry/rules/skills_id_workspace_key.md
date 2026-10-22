---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0103_workspace_scoped_foreign_keys.sql#L1-1"
    excerpt_sha256: "sha256:68ffc65d66a26a6c66967b5948de3c00458a247324b2f248fe7c13a260c2e735"
---
skills keeps unique skills_id_workspace_key: CONSTRAINT skills_id_workspace_key UNIQUE (id, workspace_id)

# Relations
- Contexts: [registry](/contexts/registry)
