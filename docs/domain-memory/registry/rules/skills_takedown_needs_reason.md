---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0013_governance.sql#L60-64"
    excerpt_sha256: "sha256:3da93915823b1768b6a0a703f005f4d39b1201d657e509c2cb597be4ac5e5c53"
---
skills keeps check skills_takedown_needs_reason: CONSTRAINT skills_takedown_needs_reason CHECK (takedown_at IS NULL OR btrim(coalesce(takedown_reason, '')) <> '')

# Relations
- Contexts: [registry](/contexts/registry)
