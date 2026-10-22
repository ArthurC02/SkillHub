---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0027_packaging.sql#L30-31"
    excerpt_sha256: "sha256:a83b861482617d72c5fa89f625359ea37fd6014d7186d9e821c714b7f17fcc8b"
---
download_artifacts keeps unique-index download_artifacts_workspace_key: CREATE UNIQUE INDEX download_artifacts_workspace_key ON download_artifacts (workspace_id, artifact_id)

# Relations
- Contexts: [packaging](/contexts/packaging)
