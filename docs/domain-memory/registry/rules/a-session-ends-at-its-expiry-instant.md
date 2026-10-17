---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/account.go#L63-68"
    excerpt_sha256: "sha256:a5d67500585fa5d20713957254af7d5b53d1c81383ddf9852c670514fb3299df"
---
A session counts only strictly before its expiry; at the expiry instant it is already signed out.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A session expiring exactly now is invalid.
