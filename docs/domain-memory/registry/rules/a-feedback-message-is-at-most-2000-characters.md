---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T15:44:47.139362Z"
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L96-100"
    excerpt_sha256: "sha256:1e6de10296c6850b4ff34b7228999f36d80c1363a6f183f98e4754ca8c4a8f00"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/beta_integration_test.go#L695-751"
    excerpt_sha256: "sha256:c382a294cb7c64621f66bbfb1e7622ccd4c23012efbe2a5e60d6e672be116dae"
---
A feedback message is trimmed, must be non-blank and may hold at most 2000 characters; otherwise it is refused with 400.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A 2001-character message answers 400; exactly 2000 is accepted.
