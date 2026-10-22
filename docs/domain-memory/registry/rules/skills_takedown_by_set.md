---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0124_skill_takedown_restore.sql#L47-48"
    excerpt_sha256: "sha256:7de1c0f559a5e04e9ef7f66f9426a3eb1d12bdf728b7e4fb00e209b0be6950d0"
---
skills keeps check skills_takedown_by_set: CONSTRAINT skills_takedown_by_set CHECK (takedown_by IS NULL OR takedown_at IS NOT NULL)

# Relations
- Contexts: [registry](/contexts/registry)
