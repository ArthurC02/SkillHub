---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/service.go#L426-431"
    excerpt_sha256: "sha256:0877d82ddf3bfc152970ec1038412bb923f5ea9c5511b7a7226a8814dced971d"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/trace_integration_test.go#L471-508"
    excerpt_sha256: "sha256:1054bd4a9dc35633329190aabd366e4ab474448c21471c898cedd6bebe520dc5"
---
The evidence an evaluation reads holds at most the last 500 events and is marked truncated and incomplete when more exist.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- A run with 1001 events gives the evaluation 500, truncated.
