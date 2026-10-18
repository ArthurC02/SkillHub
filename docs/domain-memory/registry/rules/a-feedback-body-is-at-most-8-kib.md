---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:10:47.982008Z"
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L19-22"
    excerpt_sha256: "sha256:cd43a2d1e86b25e6e2ff0a72f49b04b710d3c8a6d17f1b6d6f1123e6fb1660be"
  - resource: "apps/platform/internal/product/learning/feedback.go#L101-104"
    excerpt_sha256: "sha256:ced8c7447ff8550d084d1af336fb72253daa3eb2868ecc2fb9a2128a8dd79195"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/feedback_limits_integration_test.go#L28-40"
    excerpt_sha256: "sha256:4dd2ac094ef3e9db2b2ac19dcd5ecc68e20fe3edbeb3b28388749b278df6e38f"
---
A feedback request body over 8 KiB is refused before any field is read.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A body padded past 8 KiB answers 400 and stores nothing.
