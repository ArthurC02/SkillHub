---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L241-261"
    excerpt_sha256: "sha256:ffc78ce9143ac4a621091a8bcfce9647caaf5ad1fa552d6566ff398c6abaeb04"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L90-140"
    excerpt_sha256: "sha256:46b9015f8b3751330bbdb42a4f68b9d31445f11b10bbfa9c22e11accdd25ca30"
---
Releasing a skill for redistribution is refused when the claimed licence differs from the one its newest version records, and the stored value is unchanged.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A release claiming Apache-2.0 for a version that records MIT is refused as license_mismatch.
