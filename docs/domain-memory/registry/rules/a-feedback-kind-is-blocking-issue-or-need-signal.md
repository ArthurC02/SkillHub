---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:44:34.068147Z"
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L105-108"
    excerpt_sha256: "sha256:70eb94f8e916828cc8ff788cd220269fc1405ece7f254163469e149ecc20fa0d"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/beta_integration_test.go#L695-751"
    excerpt_sha256: "sha256:c382a294cb7c64621f66bbfb1e7622ccd4c23012efbe2a5e60d6e672be116dae"
---
A feedback kind must be blocking_issue or need_signal; any other or a missing kind is refused with 400.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- kind praise answers 400.
