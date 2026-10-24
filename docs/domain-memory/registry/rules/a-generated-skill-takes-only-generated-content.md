---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L205-207"
    excerpt_sha256: "sha256:f16cce83fddcc23cecbd24fb6c8c450a24781211818524a553627d692bb206be"
  - resource: "apps/platform/internal/skill/library/skill_root.go#L347-351"
    excerpt_sha256: "sha256:482870060733a920948cc96ea252f8d84974e64b3f502266cd8f06c3b2327f30"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L410-430"
    excerpt_sha256: "sha256:6e3a9cf26e8a82b78e43e177455988e600ecf81cc5f5a6ced03420ce3f80ce08"
---
A version of uploaded content cannot be added to a generated skill; it is refused as a name collision and consumes no version number.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Uploading a package named like a generated skill is refused.
