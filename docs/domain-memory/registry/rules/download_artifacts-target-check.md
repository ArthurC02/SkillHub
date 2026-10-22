---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0027_packaging.sql#L17-17"
    excerpt_sha256: "sha256:7a6976b075e49646afebde60aab31b7a402b5a5152c6cb7ecf5bcaa39b0ced5e"
---
download_artifacts keeps check download_artifacts.target: target text NOT NULL CHECK (btrim(target) <> '')

# Relations
- Contexts: [packaging](/contexts/packaging)
