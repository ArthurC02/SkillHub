---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/partitions.go#L17-23"
    excerpt_sha256: "sha256:167bb5b39e7db2428a9aabdca01f9184313fa1306f68786e5da08bd64a587ed5"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/trace_limits_integration_test.go#L40-59"
    excerpt_sha256: "sha256:9d1ff58e1cd00c4ce9952c78d442c79d44100b6e169ea6ef5f96f8b29b416f09"
---
Trace partition maintenance without a positive retention window is refused and deletes no events.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- Maintenance with a zero window reports an error and leaves a year-2000 event in place.
