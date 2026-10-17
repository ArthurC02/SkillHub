---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L113-116"
    excerpt_sha256: "sha256:c5769e2f3f176fe094816e80239ef453162a9bcd9328343f33d2603068dffd50"
---
A spent window allowance frees its next run one window after the oldest run it still counts.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- With a 30-day window and the oldest counted run 10 days ago, the next run frees in 20 days.
