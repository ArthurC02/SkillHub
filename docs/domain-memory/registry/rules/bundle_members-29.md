---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0088_bundles.sql#L29-29"
    excerpt_sha256: "sha256:c4ac0a75c9b280f2699e1830d09970972391d5dbab927262f87a3b93b11ce18e"
---
bundle_members keeps unique bundle_members:29: UNIQUE (bundle_version_id, manifest_name)

# Relations
- Contexts: [publishing](/contexts/publishing)
