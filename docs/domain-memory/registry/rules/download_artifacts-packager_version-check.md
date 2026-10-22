---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0027_packaging.sql#L19-19"
    excerpt_sha256: "sha256:13fc08602a27c82c4d1f65fb6d843cd02f8f796c23d22b2ed1fe26da4967f48d"
---
download_artifacts keeps check download_artifacts.packager_version: packager_version text NOT NULL CHECK (btrim(packager_version) <> '')

# Relations
- Contexts: [packaging](/contexts/packaging)
