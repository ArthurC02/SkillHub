---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L322-334"
    excerpt_sha256: "sha256:dd3f3919b96b8b5f91d42de00fb04c7f5525102a3a3a9b298e3938bf31492494"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L201-246"
    excerpt_sha256: "sha256:17c908727240d50a32ce3ff9bf0a434ad68d64f882c9c26e650dfeaec0d021aa"
---
Curating a skill is refused when it is not in a catalogue Workspace, and its tier is unchanged.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Curating a skill imported into a user's own Workspace is refused.
