---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-08T08:46:39.695261Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/http.go#L246-248"
    excerpt_sha256: "sha256:fe0cc563d110d1fb1cf68880228d8ee2eb71070a969213bef08d3460cbb077fc"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/disc_integration_test.go#L1554-1581"
    excerpt_sha256: "sha256:49c0db1962efe05d739f38b8aad5f3749d06cb601edb8ffd95ba97c504be9cee"
---
The search limit must be a whole number from 1 to 100; anything else is refused.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- limit=100 is accepted and limit=101 answers 400.
