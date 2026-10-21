---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:16:09.817174Z"
sources:
  - resource: "apps/platform/internal/creator/creation/session_start.go#L32-50"
    excerpt_sha256: "sha256:22ae849259584fe6019f5d11e7627f2e2380bf7b5883c9718fec6a5bfe48e515"
  - resource: "apps/platform/internal/creator/creation/start_test.go#L16-45"
    excerpt_sha256: "sha256:87a2f6d2cef57d471c95b6b55fe4e64e52e7f9ffe5f2fee896b9400c4047681f"
---
A session is refused before any database work when its budget is below one model call's cost or above the session ceiling.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- A budget just over the ceiling is refused as out of band.
