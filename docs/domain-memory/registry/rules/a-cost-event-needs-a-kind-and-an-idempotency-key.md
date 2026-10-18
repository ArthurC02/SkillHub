---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L66-68"
    excerpt_sha256: "sha256:709351caed8ed335c9839910a2415842814ee6a1d52f3c763d472a876aa9b230"
  - resource: "apps/platform/internal/creator/credit/refusals_test.go#L12-31"
    excerpt_sha256: "sha256:f6161c4201b988831e7e53de7d6ab4ab3e50a3db9f81f3ff0174b574eac18d39"
---
A cost event without a kind or an idempotency key is refused as invalid and nothing is recorded.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Recording a cost event with an empty idempotency key returns ErrInvalid.
