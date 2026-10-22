---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0106_search_documents_projection_integrity.sql#L10-10"
    excerpt_sha256: "sha256:423f422fb82af3d693a059787f8578f77494425d4ac07ab80cda2af92ad6c1bb"
---
skill_versions keeps unique skill_versions_id_skill_key: CONSTRAINT skill_versions_id_skill_key UNIQUE (id, skill_id)

# Relations
- Contexts: [registry](/contexts/registry)
