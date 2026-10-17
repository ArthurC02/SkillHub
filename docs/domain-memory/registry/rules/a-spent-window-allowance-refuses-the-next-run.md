---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T04:33:27.317410Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L187-192"
    excerpt_sha256: "sha256:212e583747950494d660710aa5748390f50ef5fa4ff5b44e5c1f091c21e27d4f"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L107-149"
    excerpt_sha256: "sha256:c761b7938a96c3f4cb254095afec235845ebc6501b296ed7dda2c5ec9e5fb92a"
---
A Workspace that has used its rolling-window allowance is refused with reason quota_window, and the message says when the next one frees up.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- A Workspace with 30 runs in the last 30 days but none today is refused.
