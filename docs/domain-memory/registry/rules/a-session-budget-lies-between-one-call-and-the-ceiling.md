---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:16:09.817174Z"
sources:
  - resource: "apps/platform/internal/creator/creation/session_start.go#L32-52"
    excerpt_sha256: "sha256:b20110a1f8ae1270f000a1990171cfd12ae18a370e6c5dacd0a52b9a53e30c9f"
  - resource: "apps/platform/internal/creator/creation/start_test.go#L16-45"
    excerpt_sha256: "sha256:87a2f6d2cef57d471c95b6b55fe4e64e52e7f9ffe5f2fee896b9400c4047681f"
---
A session is refused before any database work when its budget is below one model call's cost or above the session ceiling.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- A budget just over the ceiling is refused as out of band.
