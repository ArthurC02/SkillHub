---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0087_publishing.sql#L4-4"
    excerpt_sha256: "sha256:c8b96d774c05c6430141f971fb9688316c505a4f03d355395e92df35e9feb473"
---
publishers keeps unique publishers.name: name text NOT NULL UNIQUE

# Relations
- Contexts: [publishing](/contexts/publishing)
