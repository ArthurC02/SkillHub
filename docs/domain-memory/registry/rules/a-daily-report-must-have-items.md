---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:59:29.446208Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:54.156118Z"
sources:
  - resource: "apps/platform/internal/product/operations/daily_report.go#L42-44"
    excerpt_sha256: "sha256:33176ca0f95e8e4c8cf2067f496058bcd335157df4baf932bfcf59f82712130a"
  - resource: "apps/platform/internal/product/operations/daily_report_test.go#L105-147"
    excerpt_sha256: "sha256:044baace0461065579ec1c7d9564e28b3cb9e22028ceb1bda20950a9531a5bb9"
---
A daily report with no items is rejected.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A report of an empty item list is refused.
