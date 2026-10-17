---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T04:33:27.317410Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L182-186"
    excerpt_sha256: "sha256:fe837607a4ccd37e7b2399326cc71dea687a3995d3f525b67ef993c3b9964d32"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L107-149"
    excerpt_sha256: "sha256:c761b7938a96c3f4cb254095afec235845ebc6501b296ed7dda2c5ec9e5fb92a"
---
A Workspace that has used its daily allowance in the trailing 24 hours is refused with reason quota_daily, and the message says when it resets.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- A Workspace with 5 runs in the last 24 hours is refused a sixth.
