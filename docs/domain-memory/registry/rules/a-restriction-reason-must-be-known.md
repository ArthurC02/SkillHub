---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-31T05:31:14.903920Z"
sources:
  - resource: "apps/platform/internal/skill/library/governance/restriction.go#L117-131"
    excerpt_sha256: "sha256:dc58dec1c0bba20d343f1f8780efeece4343313640b9915b10c821b0048a41a4"
  - resource: "apps/platform/internal/skill/library/governance/restriction_test.go#L14-72"
    excerpt_sha256: "sha256:09350d3722923789cb55949300e57a0763c3dfee8f3b88d8f848c09a20429776"
---
Restricting a skill with a reason code outside the known set is refused, listing the known codes.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- The reason 'invented' is refused as an unknown reason code.
