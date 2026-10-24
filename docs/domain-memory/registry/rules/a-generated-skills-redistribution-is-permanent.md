---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L303-305"
    excerpt_sha256: "sha256:379f25c145143a4aa7edfded05511263d62dc7653693cc9d1450eb32343b696a"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L149-199"
    excerpt_sha256: "sha256:46b9015f8b3751330bbdb42a4f68b9d31445f11b10bbfa9c22e11accdd25ca30"
---
A generated skill's redistribution can be neither blocked nor allowed; it stays generated.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Blocking a generated skill is refused as generated_is_permanent.
