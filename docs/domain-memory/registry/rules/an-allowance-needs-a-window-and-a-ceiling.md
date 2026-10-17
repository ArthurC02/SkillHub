---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T04:33:27.317410Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L43-43"
    excerpt_sha256: "sha256:55d07fd21cfc15e1bfa77f71f46fda588492dbce6153980371900f821625bbb4"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L13-31"
    excerpt_sha256: "sha256:c45bb3d408ffb06b177a06b5a35c98dcbc8a2fb6e21a05bd7df964473a044028"
---
An allowance is enforced only when it has a window length and at least one ceiling; a zero or partial configuration refuses nothing.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- Daily 5, window 30 but no window length is not enforced.
