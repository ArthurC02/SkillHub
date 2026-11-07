---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T15:07:58.144675Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_report.go#L220-222"
    excerpt_sha256: "sha256:9fb83e409f4f0db918960f56a6fe4b7cce739cf0f7e00dbee09b071aa116093a"
  - resource: "apps/platform/internal/trial/batch/batch_test.go#L148-182"
    excerpt_sha256: "sha256:38b4d36ff1ac9750788bb312e9e57051b2694d17c3fb51659177d4d77ac9b233"
---
Only judged trials enter a pass rate; running, pending, awaiting-answer and not-evaluated trials count in neither its numerator nor its denominator.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- Four judged and two running trials give a rate over four.
