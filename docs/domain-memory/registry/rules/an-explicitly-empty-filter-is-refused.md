---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T06:26:10.741232Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/http.go#L269-307"
    excerpt_sha256: "sha256:cffc12c8fdf0ec74678f9b9bbf3b1ac54faffc4a6417c96f531360d3a7b52101"
  - resource: "apps/platform/internal/skill/discovery/facets_test.go#L266-272"
    excerpt_sha256: "sha256:8685fb51361c93ff26c582a912f603664090f3e2205483f49090ec630d6ab7d3"
---
A filter given with an empty value is refused rather than treated as absent.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- ?tier= answers 400.
