---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0088_bundles.sql#L43-46"
    excerpt_sha256: "sha256:b58887775d57bac0e758d08631aac6beed7ff771abfb3274c4fae4bb004c8c24"
---
publications keeps unique publications.bundle_id: bundle_id uuid UNIQUE REFERENCES bundles (id) ON DELETE CASCADE

# Relations
- Contexts: [publishing](/contexts/publishing)
