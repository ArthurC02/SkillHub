---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T06:26:10.741232Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/http.go#L247-257"
    excerpt_sha256: "sha256:7df1f2f1567b02af421b590a1d0806e1822000e60c2e706d9d48f452e6363800"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/disc_integration_test.go#L1444-1471"
    excerpt_sha256: "sha256:49c0db1962efe05d739f38b8aad5f3749d06cb601edb8ffd95ba97c504be9cee"
---
The search limit must be a whole number from 1 to 100; anything else is refused.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- limit=100 is accepted and limit=101 answers 400.
