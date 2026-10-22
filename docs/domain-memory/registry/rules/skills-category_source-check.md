---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0061_skill_category_source.sql#L1-5"
    excerpt_sha256: "sha256:de89259620d1c494e9a764975267377fbc3b4493027376c7fc679fc578c76a9d"
---
skills keeps check skills.category_source: category_source text CHECK (category_source IS NULL OR category_source IN ('curated', 'owner'))

# Relations
- Contexts: [registry](/contexts/registry)
