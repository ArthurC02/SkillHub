---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:14:38.912433Z"
sources:
  - resource: "apps/platform/internal/product/operations/daily_report.go#L88-98"
    excerpt_sha256: "sha256:6c93aa96ffe67c4ecb2b208ec5335013692ba2a73995b452872bd6a7f89ca0c0"
  - resource: "apps/platform/internal/product/operations/daily_report_test.go#L105-147"
    excerpt_sha256: "sha256:044baace0461065579ec1c7d9564e28b3cb9e22028ceb1bda20950a9531a5bb9"
---
A daily report item or proposal that cites no fact, or cites a pointer no tool result holds, is rejected.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- An item citing nothing fails with it cites no fact.
