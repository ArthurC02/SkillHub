---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T17:11:22.999935Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/service.go#L389-406"
    excerpt_sha256: "sha256:25c1aadf0ac80ce26114639e8b64598881c2394531a8329064ed3f64ea85ed56"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/session_lookup_integration_test.go#L56-90"
    excerpt_sha256: "sha256:a10c40994348c38c70cadebcf6c2c399af2c3b8e7e558189dd444a7610b9e2fb"
---
The Workspace carried by a signed-in request is reused only when asking about its own owner; any other account's Workspace is read from its owner.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- Inside Alice's request, asking for Bob's Workspace returns Bob's, not Alice's.
