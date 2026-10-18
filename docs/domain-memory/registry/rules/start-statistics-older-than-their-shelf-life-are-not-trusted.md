---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T08:44:17.756466Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L229-237"
    excerpt_sha256: "sha256:791cb610132d5e1c6c39e30dd0d4fcaee07b7f67f2ed96518677eddd7aafae6a"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L638-661"
    excerpt_sha256: "sha256:8c6e45e215e08486154ae89da50bca52bcd0a335c5334e2ec483daf5c749b5a3"
---
A start threshold uses recorded cost statistics only while their window ended within the statistics shelf life; older statistics give the fixed fallback, marked estimated.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Statistics whose window ended an hour past the shelf life give the fallback threshold, estimated.
