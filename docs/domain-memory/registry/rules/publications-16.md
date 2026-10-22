---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0087_publishing.sql#L16-16"
    excerpt_sha256: "sha256:8d920d1b928bff6e1848b3d70ec50dabe8c51512a840e99ddb53766366d51cae"
---
publications keeps unique publications:16: UNIQUE (publisher_id, name)

# Relations
- Contexts: [publishing](/contexts/publishing)
