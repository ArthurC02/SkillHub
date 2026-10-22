---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0088_bundles.sql#L48-54"
    excerpt_sha256: "sha256:0bbdd2977b54c9609c14cd3e62a1a50585ea9d89cb64865703b6358d0ef47b67"
---
publication_releases keeps check publication_releases_one_subject: CONSTRAINT publication_releases_one_subject CHECK (num_nonnulls(skill_version_id, bundle_version_id) = 1 AND (skill_version_id IS NULL) = (version_number IS NULL))

# Relations
- Contexts: [publishing](/contexts/publishing)
