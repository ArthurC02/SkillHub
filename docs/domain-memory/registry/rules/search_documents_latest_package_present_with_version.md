---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0106_search_documents_projection_integrity.sql#L14-25"
    excerpt_sha256: "sha256:b33a56f7378ec0de063640250d7a8edec13438a49f56f0dc953d6e97ea9bdbbf"
---
search_documents keeps check search_documents_latest_package_present_with_version: CONSTRAINT search_documents_latest_package_present_with_version CHECK ((latest_version_id IS NULL) = (latest_package_object_key IS NULL))

# Relations
- Contexts: [catalog](/contexts/catalog)
