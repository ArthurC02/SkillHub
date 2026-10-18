---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/discovery/http.go#L269-307"
    excerpt_sha256: "sha256:cffc12c8fdf0ec74678f9b9bbf3b1ac54faffc4a6417c96f531360d3a7b52101"
---
A filter given with an empty value is refused rather than treated as absent.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- ?tier= answers 400.
