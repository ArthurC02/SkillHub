---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:43:59.923381Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L113-116"
    excerpt_sha256: "sha256:c5769e2f3f176fe094816e80239ef453162a9bcd9328343f33d2603068dffd50"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L68-94"
    excerpt_sha256: "sha256:f78a678feb4911ee1a2e0b1adce32f41ebd3bb5c0b1350924f1bddcfcf46e752"
---
A spent window allowance frees its next run one window after the oldest run it still counts.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- With a 30-day window and the oldest counted run 10 days ago, the next run frees in 20 days.
