---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0130_skill_version_content_erasure.sql#L1-4"
    excerpt_sha256: "sha256:ef29e0db4dacb9f02b150d8a40d19ebc087ab51c2620e04deddb613a037bf800"
---
skill_versions keeps check skill_versions_erased_content_cleared: CONSTRAINT skill_versions_erased_content_cleared CHECK (content_erased_at IS NULL OR (manifest = '{}'::jsonb AND source_id IS NULL))

# Relations
- Contexts: [registry](/contexts/registry)
