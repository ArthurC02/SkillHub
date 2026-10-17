---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/service.go#L389-406"
    excerpt_sha256: "sha256:25c1aadf0ac80ce26114639e8b64598881c2394531a8329064ed3f64ea85ed56"
---
The Workspace carried by a signed-in request is reused only when asking about its own owner; any other account's Workspace is read from its owner.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- Inside Alice's request, asking for Bob's Workspace returns Bob's, not Alice's.
