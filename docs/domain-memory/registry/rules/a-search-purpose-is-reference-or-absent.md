---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T22:59:26.747795Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/http.go#L359-364"
    excerpt_sha256: "sha256:af25ee4a3ae972a2a9a428b288ab3d867dbd91b9849a64d8923e7c2d31e627f1"
  - resource: "apps/platform/internal/skill/discovery/http_test.go#L20-28"
    excerpt_sha256: "sha256:4af9f70860b12def7ad9cf0767a56655d0fcf7426ddf02be1a8d78df4851beab"
---
The public search refuses any purpose other than reference with 400.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- Searching with purpose=other answers 400.
