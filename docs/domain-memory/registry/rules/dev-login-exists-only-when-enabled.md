---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/http.go#L55-57"
    excerpt_sha256: "sha256:85a660fd968da5521234781c2277aba2c22190e9a7461f00a31057fa1719b90f"
---
The development login route exists only when development login is enabled; otherwise it answers 404.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- With development login off, POST /auth/dev/login answers 404.
