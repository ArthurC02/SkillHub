---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0103_workspace_scoped_foreign_keys.sql#L5-5"
    excerpt_sha256: "sha256:f2546e82d83cdbd783dce0d635ce0fc6cdc1b2ee00ddc8ce9aadcf286bc86310"
---
publication_releases keeps unique publication_releases_id_publication_key: CONSTRAINT publication_releases_id_publication_key UNIQUE (id, publication_id)

# Relations
- Contexts: [publishing](/contexts/publishing)
