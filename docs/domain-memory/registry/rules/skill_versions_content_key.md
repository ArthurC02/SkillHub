---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0003_skill_registry.sql#L45-45"
    excerpt_sha256: "sha256:0627c697bbed6fb7e07fe3a8735ecd1594f1601e3729fbcb813274bcd62c6727"
---
skill_versions keeps unique-index skill_versions_content_key: CREATE UNIQUE INDEX skill_versions_content_key ON skill_versions (skill_id, content_hash)

# Relations
- Contexts: [registry](/contexts/registry)
