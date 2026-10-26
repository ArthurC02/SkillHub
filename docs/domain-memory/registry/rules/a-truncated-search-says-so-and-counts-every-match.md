---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-26T01:48:28.064585Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/service.go#L249-252"
    excerpt_sha256: "sha256:4c012c4dfeb869b2c57bfdcfda7bd91c32c9c33ab26bada5ca244c37473f81d3"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/disc_integration_test.go#L1435-1470"
    excerpt_sha256: "sha256:04b6049045afce33da4632ddd770fe59870dfe535e3782a6d5cb5d9c9300b757"
---
Search fetches one row past the limit; when more than the limit match, it returns the limit, marks the page truncated and reports the full total.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- limit=1 over five matching skills returns one result, truncated, with total 5.
