---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0003_skill_registry.sql#L4-4"
    excerpt_sha256: "sha256:9d62c3a45b02ffbd27ecd3018dedf5eeb74cbe247cdeafe0e126da5e05b4a70d"
---
skill_sources keeps check skill_sources.source_type: source_type text NOT NULL CHECK (source_type IN ('git', 'upload'))

# Relations
- Contexts: [ingest](/contexts/ingest)
