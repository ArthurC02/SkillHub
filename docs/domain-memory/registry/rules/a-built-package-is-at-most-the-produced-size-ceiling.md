---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/delivery/packaging.go#L186-194"
    excerpt_sha256: "sha256:3084fb265faebbaadafa53fa12af62ef081e79c059ce938a4c5a90b66182200c"
---
A package the packager has built is refused when its zip exceeds the produced-size ceiling, and the refusal is counted.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A zip one byte over the ceiling is refused.
