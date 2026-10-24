---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L189-195"
    excerpt_sha256: "sha256:8b4da22b6f8785c9253aeaed44fad6d49919e2dc89690152c59aacacd179ee23"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L152-194"
    excerpt_sha256: "sha256:c761b7938a96c3f4cb254095afec235845ebc6501b296ed7dda2c5ec9e5fb92a"
---
A Workspace that has used its rolling-window allowance is refused with reason quota_window, and the message says when the next one frees up.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- A Workspace with 30 runs in the last 30 days but none today is refused.
