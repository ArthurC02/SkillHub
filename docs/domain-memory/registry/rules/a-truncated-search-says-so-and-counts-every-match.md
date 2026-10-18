---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/discovery/service.go#L247-250"
    excerpt_sha256: "sha256:4c012c4dfeb869b2c57bfdcfda7bd91c32c9c33ab26bada5ca244c37473f81d3"
---
Search fetches one row past the limit; when more than the limit match, it returns the limit, marks the page truncated and reports the full total.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- limit=1 over five matching skills returns one result, truncated, with total 5.
