---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0037_generated_skill_provenance.sql#L2-4"
    excerpt_sha256: "sha256:07b790338e073cf3d8ca230ebf3d240c2cf62be8ddac735ce3b0c0de510c749c"
---
skill_sources keeps check skill_sources_source_type_check: CONSTRAINT skill_sources_source_type_check CHECK (source_type IN ('git', 'upload', 'generated'))

# Relations
- Contexts: [ingest](/contexts/ingest)
