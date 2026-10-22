---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0106_search_documents_projection_integrity.sql#L11-12"
    excerpt_sha256: "sha256:6cfc46261b2b1c68f44c20f3d04cf7e706821123b93d1bc494677d890a6ee3fe"
---
skill_versions keeps unique skill_versions_package_key: CONSTRAINT skill_versions_package_key UNIQUE (id, skill_id, package_object_key, source_path)

# Relations
- Contexts: [registry](/contexts/registry)
