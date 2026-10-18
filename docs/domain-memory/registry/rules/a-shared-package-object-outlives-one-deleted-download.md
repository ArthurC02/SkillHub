---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T16:04:44.915924Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/download.go#L334-348"
    excerpt_sha256: "sha256:c82b77132a4b82fe96aed42ddcf0a43e47575c975ca449bbdcfe1ae78d3cbe0b"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/download_shared_object_integration_test.go#L9-33"
    excerpt_sha256: "sha256:23470d797a910cb4fa5f2d1888a9b33f7766f37779ce4590e136d30a1fd7cadb"
---
Deleting a download removes its stored object only when no other live artifact shares it.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- Deleting one of two live artifacts sharing an object keeps the object.
