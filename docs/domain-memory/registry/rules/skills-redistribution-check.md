---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0027_packaging.sql#L1-3"
    excerpt_sha256: "sha256:678c304b9c573a888f8189eb453aa1e6e2e587cf568ffb65b9291e9a040c87a2"
---
skills keeps check skills.redistribution: redistribution text NOT NULL DEFAULT 'unknown' CHECK (redistribution IN ('allowed', 'blocked', 'unknown'))

# Relations
- Contexts: [registry](/contexts/registry)
