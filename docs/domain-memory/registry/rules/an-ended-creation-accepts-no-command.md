---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/creation.go#L290-292"
    excerpt_sha256: "sha256:dde21a3732e6bedd399d7081f3edc0dbc55ff03c4a77682b60622bb26a393cf0"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L72-79"
    excerpt_sha256: "sha256:04b8fddbfbcb82716f40aa4b956fbbadecfdd0a162f534a7b78db5b7e7d89d81"
---
A saved or cancelled creation accepts no command, not even cancel.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- Cancelling a saved creation is refused as invalid.
