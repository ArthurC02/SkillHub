---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L229-232"
    excerpt_sha256: "sha256:95b01e58392589dc79e9ba8ca380c3aa282372a8f469b297f33ac9666815d81d"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L259-276"
    excerpt_sha256: "sha256:6087d7bb434fef20a4856241821a6870fcf1e2f6474fe3bec83cbe3f147baebe"
---
With fewer samples than the minimum the start threshold is the fixed fallback, marked estimated; exactly the minimum is enough.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- 19 samples give the fallback; 20 samples give the p95 threshold.
