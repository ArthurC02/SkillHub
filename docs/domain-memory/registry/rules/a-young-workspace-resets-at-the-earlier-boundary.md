---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L117-121"
    excerpt_sha256: "sha256:43b0e02377b3e68a842cd5f170d141cbe5be28368be6eed1022e64cc6f0b81ce"
---
While a Workspace is in its first window, its allowance resets at whichever comes first, the end of that first window or the rolling window's reset.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- One second before its first window ends, a Workspace whose oldest run is yesterday resets in one second.
