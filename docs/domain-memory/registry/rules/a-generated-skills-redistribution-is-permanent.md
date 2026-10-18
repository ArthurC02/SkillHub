---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L257-259"
    excerpt_sha256: "sha256:379f25c145143a4aa7edfded05511263d62dc7653693cc9d1450eb32343b696a"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L90-140"
    excerpt_sha256: "sha256:46b9015f8b3751330bbdb42a4f68b9d31445f11b10bbfa9c22e11accdd25ca30"
---
A generated skill's redistribution can be neither blocked nor allowed; it stays generated.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Blocking a generated skill is refused as generated_is_permanent.
