---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/delivery/export.go#L76-82"
    excerpt_sha256: "sha256:2f4b2a7d37b976cbe7edb1ed5693db648f1845d5a9dcf4985e3c54e93b36507c"
---
A file whose path holds a backslash or starts with a drive letter is left out of a package as an unsafe path.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- scripts\..\..\escape.sh and C:escape.sh are dropped as unsafe_path.
