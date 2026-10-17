---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/account.go#L63-68"
    excerpt_sha256: "sha256:a5d67500585fa5d20713957254af7d5b53d1c81383ddf9852c670514fb3299df"
---
An unexpired session stops counting once its account is being purged or has been deleted.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A user whose purge has started gets 401 with a still-valid cookie.
