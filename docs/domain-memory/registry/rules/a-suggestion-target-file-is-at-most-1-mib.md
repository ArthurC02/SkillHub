---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T19:36:24.856168Z"
sources:
  - resource: "apps/platform/internal/shared/skillpkg/skillpkg.go#L994-994"
    excerpt_sha256: "sha256:5dbdb75edec1a3a8e8f7fb69365c02b7fc488fa1aefe9a138059dc64921f88c6"
  - resource: "apps/platform/internal/trial/improvement/apply.go#L241-243"
    excerpt_sha256: "sha256:bb47a3c61f4d6f44328e70b203c9860b51b7773b4f3169ba7c9ba2e443ca3cdf"
  - resource: "apps/platform/internal/trial/improvement/apply_limits_characterization_test.go#L58-70"
    excerpt_sha256: "sha256:3be1479879fd1a4d545bd19eebc128f5d7ee995e105eaafc9d8c8f908565e31b"
---
A file a suggestion targets is read for a reviewable diff only up to 1 MiB; a larger file is refused before any content check.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A file of exactly 1048576 bytes is read; 1048577 bytes is refused as larger than the byte limit for a reviewable diff.
