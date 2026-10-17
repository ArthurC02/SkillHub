---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:43:59.923381Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L95-100"
    excerpt_sha256: "sha256:7272d86d1cd1a395b370a6918e41b68d8f311fae3eab380bfb071c7dfed3035b"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L152-194"
    excerpt_sha256: "sha256:c761b7938a96c3f4cb254095afec235845ebc6501b296ed7dda2c5ec9e5fb92a"
---
The window allowance counts the runs of the configured number of days before now, a rolling window rather than a calendar month.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- With a 30-day window, a run 29 days ago counts and one 31 days ago does not.
