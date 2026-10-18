---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L112-114"
    excerpt_sha256: "sha256:2496f84c4da33eafa5b496a8012dbfcbe62e13a9541237f2e94a393ba88eac62"
  - resource: "apps/platform/internal/creator/credit/refusals_test.go#L33-53"
    excerpt_sha256: "sha256:23227243f6f0bbc82a85e42254fea3f8cf6125eb3cfce5d3e261985c951c06c8"
---
A charge without a kind, an idempotency key or a user is refused as invalid.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- A charge with an empty kind returns ErrInvalid.
