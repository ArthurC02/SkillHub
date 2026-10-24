---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/packaging.go#L185-193"
    excerpt_sha256: "sha256:3084fb265faebbaadafa53fa12af62ef081e79c059ce938a4c5a90b66182200c"
  - resource: "apps/platform/internal/skill/delivery/size_test.go#L18-34"
    excerpt_sha256: "sha256:b1c0fce60b779b1f8ad90e845409eaa513c093e66cccac9641a2512800f0b50c"
---
A package the packager has built is refused when its zip exceeds the produced-size ceiling, and the refusal is counted.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A zip one byte over the ceiling is refused.
