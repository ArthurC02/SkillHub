---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L22-24"
    excerpt_sha256: "sha256:785d4d27d5fd6f7f472a438061c89af19811523dfa488642014a87660cb170bc"
  - resource: "apps/platform/internal/product/learning/feedback.go#L89-91"
    excerpt_sha256: "sha256:19788726fe2527badae7460deb272c74db88567dfde7ca77a03616406c8086cc"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/feedback_limits_integration_test.go#L28-40"
    excerpt_sha256: "sha256:4dd2ac094ef3e9db2b2ac19dcd5ecc68e20fe3edbeb3b28388749b278df6e38f"
---
A feedback request body over 8 KiB is refused before any field is read.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A body padded past 8 KiB answers 400 and stores nothing.
