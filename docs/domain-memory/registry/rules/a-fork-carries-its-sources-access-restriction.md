---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L190-199"
    excerpt_sha256: "sha256:406ffe570d5690630bf40d018bf93c4f14a78a6aef4bafbe9cc2866b63bcf4d2"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/skill_hardening_a5_integration_test.go#L16-56"
    excerpt_sha256: "sha256:4373462396d014b7085cb48c6594ceb199a32793981b7f128c259f164128acdc"
---
A fork starts with its source's access restriction, so forking cannot get around a licensing hold.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Forking a skill held for licence review gives a held fork whose files answer 403.
