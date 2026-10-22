---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0088_bundles.sql#L17-17"
    excerpt_sha256: "sha256:5b33ae3e33a3b66af283389d41a36b23e80c183bb3939850b97c2b5ef6e82c42"
---
bundle_versions keeps unique bundle_versions:17: UNIQUE (bundle_id, version)

# Relations
- Contexts: [publishing](/contexts/publishing)
