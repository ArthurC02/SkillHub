---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/session_start.go#L32-52"
    excerpt_sha256: "sha256:b20110a1f8ae1270f000a1990171cfd12ae18a370e6c5dacd0a52b9a53e30c9f"
---
A session is refused before any database work when its budget is below one model call's cost or above the session ceiling.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- A budget just over the ceiling is refused as out of band.
