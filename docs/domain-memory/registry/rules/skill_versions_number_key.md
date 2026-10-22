---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0003_skill_registry.sql#L44-44"
    excerpt_sha256: "sha256:d59a801e53843c01532e97928f9ae53016edc56a75cdd767b960fc0694d9570d"
---
skill_versions keeps unique-index skill_versions_number_key: CREATE UNIQUE INDEX skill_versions_number_key ON skill_versions (skill_id, version_number)

# Relations
- Contexts: [registry](/contexts/registry)
