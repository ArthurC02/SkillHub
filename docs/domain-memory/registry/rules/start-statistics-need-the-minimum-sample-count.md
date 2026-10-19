---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-19T17:32:50.132592Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L231-236"
    excerpt_sha256: "sha256:0685ecaea8cd02a3f5f28d1fd035c2ed0ab404b27bc26b8b71dd25ad07cc7785"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L259-276"
    excerpt_sha256: "sha256:6087d7bb434fef20a4856241821a6870fcf1e2f6474fe3bec83cbe3f147baebe"
---
With fewer samples than the minimum the start threshold is the fixed fallback, marked estimated; exactly the minimum is enough.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- 19 samples give the fallback; 20 samples give the p95 threshold.
