---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:43:59.923381Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L117-121"
    excerpt_sha256: "sha256:43b0e02377b3e68a842cd5f170d141cbe5be28368be6eed1022e64cc6f0b81ce"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L96-127"
    excerpt_sha256: "sha256:080c648b367e9a099998601bf11f09d5af67bae5e1cdd44bed09aa11ce8a01ea"
---
While a Workspace is in its first window, its allowance resets at whichever comes first, the end of that first window or the rolling window's reset.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- One second before its first window ends, a Workspace whose oldest run is yesterday resets in one second.
