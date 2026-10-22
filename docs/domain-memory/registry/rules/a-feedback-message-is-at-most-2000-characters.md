---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T13:08:10.780011Z"
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L95-99"
    excerpt_sha256: "sha256:740f6a9b5ad7f425f235474b5e08be2228b540cca89034daf4a3513e1ee0cda0"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/beta_integration_test.go#L695-751"
    excerpt_sha256: "sha256:c382a294cb7c64621f66bbfb1e7622ccd4c23012efbe2a5e60d6e672be116dae"
---
A feedback message is trimmed, must be non-blank and may hold at most 2000 characters; otherwise it is refused with 400.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A 2001-character message answers 400; exactly 2000 is accepted.
