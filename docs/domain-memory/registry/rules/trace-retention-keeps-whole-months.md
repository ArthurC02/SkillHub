---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:04:37.509165Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:53.057386Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/partitions.go#L17-23"
    excerpt_sha256: "sha256:167bb5b39e7db2428a9aabdca01f9184313fa1306f68786e5da08bd64a587ed5"
  - resource: "apps/platform/internal/trial/evidence/partitions.go#L34-37"
    excerpt_sha256: "sha256:f24c8367cf2ef2bff598700b70d73853238997e0c3e8bf9d7e62cf4b34b138d6"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/trace_retention_integration_test.go#L11-54"
    excerpt_sha256: "sha256:86db42c2cd414abe3cfbac87bc64ca77ca3547edc927f9b3fbd3f7ffd7012f86"
---
Trace maintenance deletes events older than the first day of the month holding the retention cutoff; events of that month stay.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- With a cutoff in mid January, an event on 31 December is deleted and one on 1 January stays.
