---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:43:59.923381Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L182-192"
    excerpt_sha256: "sha256:9afc7ba25773b59264f1e73efa25c1af874f1586f0ef21cc7a7262006d8b919f"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L142-150"
    excerpt_sha256: "sha256:d2f634cab52cfcf0202b2c705180959fea8a34c7b5316848535c8e12bfe38625"
---
When the daily and the window allowance are both spent, the refusal names the daily allowance and when it resets.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- Five runs today and thirty this window refuse with quota_daily and its 24-hour reset.
