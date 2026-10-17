---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/write.go#L17-22"
    excerpt_sha256: "sha256:cd9bbe04b7739c927a4a19310eede44f46be6bb304e63d0fd047a43034e0f11b"
---
A package whose validation report is blocked or carries no manifest cannot become a Skill or a version.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A report with Blocked set is refused with ErrUnvalidatedPackage.
