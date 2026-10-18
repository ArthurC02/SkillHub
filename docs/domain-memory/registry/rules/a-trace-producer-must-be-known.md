---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/event.go#L133-134"
    excerpt_sha256: "sha256:5f5a68c8d0f5101d5794a037e18481269171149616d35d0d60ff2d2c0ec358c4"
  - resource: "apps/platform/internal/trial/evidence/trace_test.go#L175-214"
    excerpt_sha256: "sha256:e8af8b0f200535df46a9c978e734a7f0de0d6fa35668e2dedfb58ab3c7ce26d2"
---
An event whose emitted_by is not a known producer is refused.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- An event emitted by 'browser' is refused as invalid.
