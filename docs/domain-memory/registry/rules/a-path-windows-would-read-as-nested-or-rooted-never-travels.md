---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-11T17:10:17.837079Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/export.go#L78-84"
    excerpt_sha256: "sha256:6809fe0290eb6f6525a084edd8ba852e1267703d18d80f2bf8108af187cc0a61"
  - resource: "apps/platform/internal/skill/delivery/export_path_test.go#L9-31"
    excerpt_sha256: "sha256:f918b61117ac9ad181e028ab7bc46f393317ca5b5e13e10efaf58fc8d8ac1bf7"
---
A file whose path holds a backslash or starts with a drive letter is left out of a package as an unsafe path.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- scripts\..\..\escape.sh and C:escape.sh are dropped as unsafe_path.
