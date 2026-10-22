---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0023_access_restriction.sql#L1-3"
    excerpt_sha256: "sha256:b5d1a9f00577588f64c3ad19c8c41bb0ad837eea8b7461b3434e4671087880c0"
---
skills keeps check skills.access_restriction: access_restriction text CHECK (access_restriction IS NULL OR btrim(access_restriction) <> '')

# Relations
- Contexts: [registry](/contexts/registry)
