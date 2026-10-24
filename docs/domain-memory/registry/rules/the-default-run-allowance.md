---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L17-22"
    excerpt_sha256: "sha256:58b8248ef779eed06f35dfbc8dd2ff4aaf70e45209cea13366d98c544af7588e"
  - resource: "apps/platform/internal/product/entitlements/quota_test.go#L35-44"
    excerpt_sha256: "sha256:0e69279ca7382a95a8ae0b485b7b9a01f6862a1cbff41010e0b508578e3af8bd"
---
By default a Workspace may start 5 runs a day and 30 per 30 days, and 20 in its first 30 days.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- An unconfigured deployment refuses a sixth run in one day.
