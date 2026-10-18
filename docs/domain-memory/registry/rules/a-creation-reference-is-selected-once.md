---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T20:37:17.368504Z"
sources:
  - resource: "apps/platform/internal/creator/creation/reference_commands.go#L36-38"
    excerpt_sha256: "sha256:05ebe0b488f5c07a9f9027de57a98b16ff57519d3679404f6492dc662742b37b"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L230-255"
    excerpt_sha256: "sha256:ba81276384498fe0aae8aac8011078e6f03e21e4cc614077aaee7ee93862732e"
---
Selecting the same reference Skill twice is refused.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- Selecting one skill twice is refused as invalid.
