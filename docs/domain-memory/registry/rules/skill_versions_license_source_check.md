---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0014_license_manifest_reference.sql#L4-8"
    excerpt_sha256: "sha256:2f4486ab8d2b666c8b7f9a7b14803524a4a8e7be57c55e679f773c26f9d70f06"
---
skill_versions keeps check skill_versions_license_source_check: CONSTRAINT skill_versions_license_source_check CHECK (license_source IN ( 'manifest', 'manifest-referenced-file', 'package-license-file', 'repo-license-file' ))

# Relations
- Contexts: [registry](/contexts/registry)
