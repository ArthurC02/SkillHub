---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/packaging.go#L972-991"
    excerpt_sha256: "sha256:0fe2028033dac70765a220a20543fd38a5cdf092e48f536b0c1c71cd62fbae71"
  - resource: "apps/platform/internal/skill/delivery/source_rules_test.go#L46-54"
    excerpt_sha256: "sha256:ae062c864aabbcf0ea8f66a6fab575cbf5bf7c1895a1a22f6d83381ef9820c92"
---
A package is refused when SKILL.md points at a file or directory the packager leaves out, since the download would miss what its own instructions use.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- SKILL.md linking node_modules/helper.js blocks the package as file_removed_by_packager.
