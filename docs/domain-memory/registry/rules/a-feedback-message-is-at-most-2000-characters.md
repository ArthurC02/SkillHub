---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:44:34.068147Z"
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L109-113"
    excerpt_sha256: "sha256:c6aab9ace0f515da994f241b75d035e9c9e248dec2beeddc5b86d1c1069d94b2"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/beta_integration_test.go#L695-751"
    excerpt_sha256: "sha256:c382a294cb7c64621f66bbfb1e7622ccd4c23012efbe2a5e60d6e672be116dae"
---
A feedback message is trimmed, must be non-blank and may hold at most 2000 characters; otherwise it is refused with 400.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A 2001-character message answers 400; exactly 2000 is accepted.
