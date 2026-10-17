---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:16:09.817174Z"
sources:
  - resource: "apps/platform/internal/creator/creation/creation.go#L285-308"
    excerpt_sha256: "sha256:e620da6d930b20f109a6417a623ce6a8aaa563895900dba269b812192ab9a5e6"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L89-99"
    excerpt_sha256: "sha256:fef8105ca0d1d2d64c1c6a69b39b14185821def4d6fd6dd9ffa23277de398d51"
---
After a creation session's deadline every command except cancel is refused.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- A message after the deadline is refused; cancel still works.
