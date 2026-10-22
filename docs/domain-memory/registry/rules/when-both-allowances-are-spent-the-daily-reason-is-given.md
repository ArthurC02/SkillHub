---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T19:48:59.631039Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L182-195"
    excerpt_sha256: "sha256:2779ac232d676eefbe2539c47bed6031bd3282a831bd080e3aa4acb5ec1b0281"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L142-150"
    excerpt_sha256: "sha256:d2f634cab52cfcf0202b2c705180959fea8a34c7b5316848535c8e12bfe38625"
---
When the daily and the window allowance are both spent, the refusal names the daily allowance and when it resets.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- Five runs today and thirty this window refuse with quota_daily and its 24-hour reset.
