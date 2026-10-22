---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0027_packaging.sql#L21-21"
    excerpt_sha256: "sha256:53c5012f706563409e3565182353c75d2862ca88f6a8c43c4fc4d0acd85a2e94"
---
download_artifacts keeps check download_artifacts.manifest_hash: manifest_hash text NOT NULL CHECK (btrim(manifest_hash) <> '')

# Relations
- Contexts: [packaging](/contexts/packaging)
