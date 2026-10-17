---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T17:12:31.935298Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/export.go#L76-82"
    excerpt_sha256: "sha256:2f4b2a7d37b976cbe7edb1ed5693db648f1845d5a9dcf4985e3c54e93b36507c"
  - resource: "apps/platform/internal/skill/delivery/export_path_test.go#L9-31"
    excerpt_sha256: "sha256:f918b61117ac9ad181e028ab7bc46f393317ca5b5e13e10efaf58fc8d8ac1bf7"
---
A file whose path holds a backslash or starts with a drive letter is left out of a package as an unsafe path.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- scripts\..\..\escape.sh and C:escape.sh are dropped as unsafe_path.
