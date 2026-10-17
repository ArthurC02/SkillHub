---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/entitlements/retention.go#L12-17"
    excerpt_sha256: "sha256:596b19b4e090d0f1897f7939b6fdf9378f2e28bead414ca390ac8e2ea7ef1e93"
  - resource: "apps/platform/internal/product/entitlements/retention_test.go#L9-19"
    excerpt_sha256: "sha256:16cfefa237c71e16ab9d419a8d465a6980d5e743882e93b844d4551b39ef8bf6"
---
Download retention must be a positive duration; zero or negative is refused with ErrRetentionNotConfigured rather than meaning keep forever or delete at once.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- A deployment with retention 0 cannot compute a retention period.
