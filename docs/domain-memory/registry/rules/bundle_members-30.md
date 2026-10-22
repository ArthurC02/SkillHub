---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0088_bundles.sql#L30-30"
    excerpt_sha256: "sha256:8f7d41e36d0ce9ba3e23457e2093db025f871357643a383db748b54e4a2b997d"
---
bundle_members keeps unique bundle_members:30: UNIQUE (bundle_version_id, position)

# Relations
- Contexts: [publishing](/contexts/publishing)
