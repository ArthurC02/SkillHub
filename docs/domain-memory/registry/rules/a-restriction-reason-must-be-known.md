---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/library/governance/restriction.go#L117-131"
    excerpt_sha256: "sha256:dc58dec1c0bba20d343f1f8780efeece4343313640b9915b10c821b0048a41a4"
  - resource: "apps/platform/internal/skill/library/governance/restriction_test.go#L14-72"
    excerpt_sha256: "sha256:4bacb67d4339c6faa0522e958183e8b95ca051dc0d555f57a1621f89f47da8e6"
---
Restricting a skill with a reason code outside the known set is refused, listing the known codes.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- The reason 'invented' is refused as an unknown reason code.
