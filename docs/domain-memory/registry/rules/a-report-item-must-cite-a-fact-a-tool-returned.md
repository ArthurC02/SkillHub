---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/operations/daily_report.go#L88-98"
    excerpt_sha256: "sha256:6c93aa96ffe67c4ecb2b208ec5335013692ba2a73995b452872bd6a7f89ca0c0"
---
A daily report item or proposal that cites no fact, or cites a pointer no tool result holds, is rejected.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- An item citing nothing fails with it cites no fact.
