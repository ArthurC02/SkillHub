---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0027_packaging.sql#L18-18"
    excerpt_sha256: "sha256:c8bf3986a73669bb2b99bf1fde2e96b0867395f0d31eb4ddaddcc822f7799d2f"
---
download_artifacts keeps check download_artifacts.profile_version: profile_version text NOT NULL CHECK (btrim(profile_version) <> '')

# Relations
- Contexts: [packaging](/contexts/packaging)
