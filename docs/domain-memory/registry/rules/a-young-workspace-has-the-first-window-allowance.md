---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L106-121"
    excerpt_sha256: "sha256:570a7c58898fba68b1e51679006ab7919ce909d0d9bc46ae3958c3862faaf50c"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L96-127"
    excerpt_sha256: "sha256:080c648b367e9a099998601bf11f09d5af67bae5e1cdd44bed09aa11ce8a01ea"
---
Until a Workspace's first window ends, its window ceiling is the first-window allowance, and its window resets at the earlier of that end and the rolling reset; from the moment the first window ends the ordinary window ceiling applies.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- One second before its first 30 days end a Workspace's ceiling is 20; at the moment they end it is 30.
