---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T17:11:22.999935Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/account.go#L63-68"
    excerpt_sha256: "sha256:a5d67500585fa5d20713957254af7d5b53d1c81383ddf9852c670514fb3299df"
  - resource: "apps/platform/internal/creator/workspace/account_test.go#L11-32"
    excerpt_sha256: "sha256:0f9ff53abbd7fad80898ad4d6a6d0b83e07318eb807954a08e0bfcc68276a44e"
---
An unexpired session stops counting once its account is being purged or has been deleted.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A user whose purge has started gets 401 with a still-valid cookie.
