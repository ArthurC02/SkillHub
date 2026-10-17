---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T01:25:37.610091Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/service.go#L239-239"
    excerpt_sha256: "sha256:43f8454b95f77d0bbe30de34cd064d82049b9218d1218a83b52cd3d09fab88d2"
  - resource: "apps/platform/internal/creator/workspace/service.go#L272-272"
    excerpt_sha256: "sha256:af715aa9ad0a5d6a90a7c9cdb7bbc0c5856e449f7fa9a46cf98fced866766050"
  - resource: "apps/platform/internal/creator/workspace/service.go#L463-466"
    excerpt_sha256: "sha256:3ac2b38361fc85de9017f77d0bd9ab98206ee29283cf301bfc9f3ad41bd9711f"
  - resource: "apps/platform/internal/creator/workspace/signup_integration_test.go#L98-123"
    excerpt_sha256: "sha256:6c1dc4919a0831b97ec1747dec4c592c41320acf48d1d41155e5bd964062f9ac"
---
The raw session token exists only in the cookie handed to the user; the platform stores and looks up only its SHA-256 hash.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- Reading the sessions table never reveals a token that can be replayed as a cookie.
