---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/http.go#L78-81"
    excerpt_sha256: "sha256:b9e0f9c6d0e2437cefe5fbc3de87db6d27321918a57f92a4d6a60e4d1bc7ca70"
  - resource: "apps/platform/internal/creator/workspace/http_test.go#L35-50"
    excerpt_sha256: "sha256:434ad64af8102470abc001f76704971f08b61c83a80582c1fae6c20d47782ab7"
---
A development login name longer than 64 characters is refused with 400 before any account is touched.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A 64-character name logs in; 65 characters answer 400.
