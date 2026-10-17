---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T04:33:27.317410Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L135-141"
    excerpt_sha256: "sha256:b7f7b8f3bc7490a633e92bd1556c9b56b01070b8cb31dabc0e944b3a9b83ca00"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L44-51"
    excerpt_sha256: "sha256:ab02a82bd1ec1fdfb6e16bf14d99239cac78a9cd3a3dac983568fd08d8c987f0"
---
The remaining allowance a Workspace sees is clamped at zero; an overspent Workspace reads 0, not a negative number.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- Limit 5 with 7 used reads 0 remaining.
