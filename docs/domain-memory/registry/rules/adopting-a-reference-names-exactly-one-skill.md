---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/creation/reference_commands.go#L56-57"
    excerpt_sha256: "sha256:9d6e71be2077ba415b2c4343f7c3e0ab7c0688f705dd786832da816167dd653f"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L276-309"
    excerpt_sha256: "sha256:60cd4412071745c1d42626f208e05f073c4420dd3c2eb4316cc6f8c03119283b"
---
Adopting a reference names exactly one listed Skill.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- Adopting two skills at once is refused.
