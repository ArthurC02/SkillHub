---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:43:00.552336Z"
sources:
  - resource: "apps/platform/internal/skill/library/write.go#L17-22"
    excerpt_sha256: "sha256:cd9bbe04b7739c927a4a19310eede44f46be6bb304e63d0fd047a43034e0f11b"
  - resource: "apps/platform/internal/skill/library/write_test.go#L12-30"
    excerpt_sha256: "sha256:10803ff72e20ae17ca2a98da8b1fdf41d396922e48f9714ff4367cab87a3af23"
---
A package whose validation report is blocked or carries no manifest cannot become a Skill or a version.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A report with Blocked set is refused with ErrUnvalidatedPackage.
