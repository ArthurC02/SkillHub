---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0012_license_provenance.sql#L1-3"
    excerpt_sha256: "sha256:4dea29e4dad81b7d283523e7796a79864b83b9099336bdeb709815a41b5ef37f"
---
skill_versions keeps check skill_versions.license_source: license_source text CHECK (license_source IN ('manifest', 'package-license-file', 'repo-license-file'))

# Relations
- Contexts: [registry](/contexts/registry)
