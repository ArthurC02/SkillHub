---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch_report.go#L220-222"
    excerpt_sha256: "sha256:9fb83e409f4f0db918960f56a6fe4b7cce739cf0f7e00dbee09b071aa116093a"
  - resource: "apps/platform/internal/trial/batch/batch_test.go#L147-181"
    excerpt_sha256: "sha256:24cd351a1c08ff938035fb610be92d984aef28aed25026fc63801de5d9114834"
---
Only judged trials enter a pass rate; running, pending, awaiting-answer and not-evaluated trials count in neither its numerator nor its denominator.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- Four judged and two running trials give a rate over four.
