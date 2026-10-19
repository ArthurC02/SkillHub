---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-19T17:32:50.132592Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L231-238"
    excerpt_sha256: "sha256:fb76a7ad3c282a89b62e5a51a3b1f2acf6e5df6ed6373d206eef863088873e23"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L638-661"
    excerpt_sha256: "sha256:8c6e45e215e08486154ae89da50bca52bcd0a335c5334e2ec483daf5c749b5a3"
---
A start threshold uses recorded cost statistics only while their window ended within the statistics shelf life; older statistics give the fixed fallback, marked estimated.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Statistics whose window ended an hour past the shelf life give the fallback threshold, estimated.
