---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L389-395"
    excerpt_sha256: "sha256:9e57aa60aaac878cda5e538c489cdd412317eae73ab47d644347b12aafaa4834"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L512-536"
    excerpt_sha256: "sha256:c9d3a601965214aa804f9e09b878002d4470772cec1cfc93719b40388a71190a"
---
Recomputing statistics sweeps idle session summaries first only for the creation session statistic and only when an idle window is configured.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Recomputing the creation step statistic sweeps no sessions.
