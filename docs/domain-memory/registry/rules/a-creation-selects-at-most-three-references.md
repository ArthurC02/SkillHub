---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/reference_commands.go#L27-29"
    excerpt_sha256: "sha256:7d268eae9083b828d8aa2d17d32c03d9ed96e72ffddc0355afe5e4d6d4a8ec36"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L230-255"
    excerpt_sha256: "sha256:ba81276384498fe0aae8aac8011078e6f03e21e4cc614077aaee7ee93862732e"
---
Selecting more than three reference Skills is refused.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- Selecting four references is refused as invalid.
