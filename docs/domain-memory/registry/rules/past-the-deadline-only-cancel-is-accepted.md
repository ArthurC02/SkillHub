---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/creation.go#L285-308"
    excerpt_sha256: "sha256:e620da6d930b20f109a6417a623ce6a8aaa563895900dba269b812192ab9a5e6"
---
After a creation session's deadline every command except cancel is refused.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- A message after the deadline is refused; cancel still works.
