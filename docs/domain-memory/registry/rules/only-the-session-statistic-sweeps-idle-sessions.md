---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L403-409"
    excerpt_sha256: "sha256:9e57aa60aaac878cda5e538c489cdd412317eae73ab47d644347b12aafaa4834"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L551-575"
    excerpt_sha256: "sha256:c9d3a601965214aa804f9e09b878002d4470772cec1cfc93719b40388a71190a"
---
Recomputing statistics sweeps idle session summaries first only for the creation session statistic and only when an idle window is configured.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Recomputing the creation step statistic sweeps no sessions.
