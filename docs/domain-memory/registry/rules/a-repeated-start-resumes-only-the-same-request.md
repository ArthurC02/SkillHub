---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/session_start.go#L54-64"
    excerpt_sha256: "sha256:828dbb63dd45e30cd26b34959b781dbb49311e3809abf9a8289690d5f55da76c"
---
Starting a session with an id already used returns that session only for the same message and budget; a different request with the same id is refused as a replay mismatch.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- The same id with another message is refused.
