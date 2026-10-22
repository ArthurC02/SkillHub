---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0053_skill_category.sql#L1-3"
    excerpt_sha256: "sha256:cf59d8003007adf13249cefd8118de5e00c3c658177089dc91cfb19fbe165554"
---
skills keeps check skills.category: category text CHECK (category IS NULL OR category IN ('documents', 'writing', 'data'))

# Relations
- Contexts: [registry](/contexts/registry)
