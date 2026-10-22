---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T19:48:59.631039Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L101-101"
    excerpt_sha256: "sha256:1797f3509bbc09245a741eb54d357af291c9c7282d56820b78049db85fe4efb6"
  - resource: "apps/platform/internal/product/entitlements/quota.go#L182-188"
    excerpt_sha256: "sha256:5f5171a7cd49e570c7ec22e90eae40e17639f4631d3337805e5239e550b50adc"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L152-194"
    excerpt_sha256: "sha256:c761b7938a96c3f4cb254095afec235845ebc6501b296ed7dda2c5ec9e5fb92a"
---
The daily allowance counts the runs of the 24 hours before now, not of the calendar day.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- A run at 23:00 yesterday still counts at 10:00 today.
