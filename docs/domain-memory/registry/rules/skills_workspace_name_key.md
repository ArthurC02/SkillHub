---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0008_skill_deletion.sql#L4-5"
    excerpt_sha256: "sha256:65bef04fa37be68a196d5e7703a4d69b5c30d7d5ad200e5d62347d1cc177ed19"
---
skills keeps unique-index skills_workspace_name_key: CREATE UNIQUE INDEX skills_workspace_name_key ON skills (workspace_id, name) WHERE deleted_at IS NULL

# Relations
- Contexts: [registry](/contexts/registry)
