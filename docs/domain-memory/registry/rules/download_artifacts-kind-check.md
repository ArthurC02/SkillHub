---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0027_packaging.sql#L12-12"
    excerpt_sha256: "sha256:98883a7118f7425fb6231f256c418516a7b63771ac664f4b0c85905bebba035c"
---
download_artifacts keeps check download_artifacts.kind: kind text NOT NULL DEFAULT 'download_package' CHECK (kind = 'download_package')

# Relations
- Contexts: [packaging](/contexts/packaging)
