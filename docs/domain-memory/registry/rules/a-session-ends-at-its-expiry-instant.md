---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/account.go#L75-80"
    excerpt_sha256: "sha256:a5d67500585fa5d20713957254af7d5b53d1c81383ddf9852c670514fb3299df"
  - resource: "apps/platform/internal/creator/workspace/account_test.go#L11-32"
    excerpt_sha256: "sha256:0f9ff53abbd7fad80898ad4d6a6d0b83e07318eb807954a08e0bfcc68276a44e"
---
A session counts only strictly before its expiry; at the expiry instant it is already signed out.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A session expiring exactly now is invalid.
