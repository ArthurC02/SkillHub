---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T19:48:59.631039Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L182-188"
    excerpt_sha256: "sha256:5f5171a7cd49e570c7ec22e90eae40e17639f4631d3337805e5239e550b50adc"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L107-149"
    excerpt_sha256: "sha256:c761b7938a96c3f4cb254095afec235845ebc6501b296ed7dda2c5ec9e5fb92a"
---
A Workspace that has used its daily allowance in the trailing 24 hours is refused with reason quota_daily, and the message says when it resets.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- A Workspace with 5 runs in the last 24 hours is refused a sixth.
