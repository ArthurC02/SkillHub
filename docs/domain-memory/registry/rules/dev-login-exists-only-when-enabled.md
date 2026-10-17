---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T17:11:22.999935Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/http.go#L55-57"
    excerpt_sha256: "sha256:85a660fd968da5521234781c2277aba2c22190e9a7461f00a31057fa1719b90f"
  - resource: "apps/platform/internal/creator/workspace/github_test.go#L240-263"
    excerpt_sha256: "sha256:c3331dac3482412666a5beadd2f4a7c1a8666f465a0d826b8522b789d95cf2b9"
---
The development login route exists only when development login is enabled; otherwise it answers 404.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- With development login off, POST /auth/dev/login answers 404.
