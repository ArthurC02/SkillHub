---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L175-181"
    excerpt_sha256: "sha256:7f67e78ff28ff673883a8d7a6a02357ee6e8e429dd961e50884f253e5d19381c"
  - resource: "apps/platform/internal/product/entitlements/generate_quota_test.go#L59-94"
    excerpt_sha256: "sha256:04449d826f1fcb489d91b5c31eb6816e0826e73ac108a3f8aa02787a78fe660b"
---
When usage cannot be counted the request is refused with ErrAllowanceUnavailable and a *_unavailable reason; it is never read as unused and never reported as exhausted.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- The usage lookup fails, so a generation is refused as generate_quota_unavailable.
