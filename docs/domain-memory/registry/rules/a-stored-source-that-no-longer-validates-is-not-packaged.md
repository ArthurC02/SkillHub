---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T16:04:44.915924Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/packaging.go#L957-966"
    excerpt_sha256: "sha256:661cdef401ae1f23569c32f6603071f7cae8f34fd9be7310382fbbd3693f4a60"
  - resource: "apps/platform/internal/skill/delivery/source_rules_test.go#L66-71"
    excerpt_sha256: "sha256:2b807a32bc2f142f984aab60d2286bc21c7e861a4521ebbe606a9865dd9ca83f"
---
A stored source that now fails import validation is not packaged, except for errors inside a directory the packager leaves out anyway.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A SKILL.md without front matter blocks the package as validation_blocked.
