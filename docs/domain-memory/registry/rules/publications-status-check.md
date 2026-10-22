---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0087_publishing.sql#L13-13"
    excerpt_sha256: "sha256:fb145d17a145e2b6a8c3f93e7358d34caf036e0145a2d3733baf176f7390d1e5"
---
publications keeps check publications.status: status text NOT NULL CHECK (status IN ('published', 'delisted'))

# Relations
- Contexts: [publishing](/contexts/publishing)
