---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T20:37:17.368504Z"
sources:
  - resource: "apps/platform/internal/creator/creation/tool_actions.go#L49-51"
    excerpt_sha256: "sha256:52e9a5f1f3f0a6f22e1cbf561f5f2a7dccae6d35bd80a5b9b06b88b4f274430c"
  - resource: "apps/platform/internal/creator/creation/creation_test.go#L615-623"
    excerpt_sha256: "sha256:97976b97b4da3bcffee4a5b73675bbf3fd3162873a7bbf7c969354a244770aba"
---
A creation searches the catalogue at most twice; a third search is answered with a note and searches nothing.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- After two empty rounds a third search is not run.
