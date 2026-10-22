---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0012_license_provenance.sql#L11-13"
    excerpt_sha256: "sha256:b6b7d432aa62a372a6644af623152351c76a40de02176e93f7f54131af9ff2b9"
---
skill_versions keeps check skill_versions_license_source_pairing: CONSTRAINT skill_versions_license_source_pairing CHECK ((license_expression IS NULL) = (license_source IS NULL)) NOT VALID

# Relations
- Contexts: [registry](/contexts/registry)
