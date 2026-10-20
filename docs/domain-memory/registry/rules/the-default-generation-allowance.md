---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-20T14:04:15.090038Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/generate_quota.go#L13-16"
    excerpt_sha256: "sha256:cdd69479876b353ad4abdf901863c829941f5e8d405942d32e01a3ddf0f75709"
  - resource: "apps/platform/internal/product/entitlements/generate_quota_test.go#L96-107"
    excerpt_sha256: "sha256:4caee688f7390124fa5dd29c55f94b0e17d841951cb3c5915f1792ff999e26e7"
---
By default a Workspace may generate 10 skills a day and 30 per 30 days, and 20 in its first 30 days; the daily generation allowance is higher than the daily run allowance.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- An unconfigured deployment refuses an eleventh generation in one day.
