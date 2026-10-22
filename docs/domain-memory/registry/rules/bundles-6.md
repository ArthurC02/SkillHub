---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0088_bundles.sql#L6-6"
    excerpt_sha256: "sha256:d58b763e85272b639ac05e2c1439eae8cb6202b93ef1c59493df06820577c821"
---
bundles keeps unique bundles:6: UNIQUE (workspace_id, name)

# Relations
- Contexts: [publishing](/contexts/publishing)
