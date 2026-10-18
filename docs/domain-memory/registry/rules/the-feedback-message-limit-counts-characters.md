---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L109-113"
    excerpt_sha256: "sha256:c6aab9ace0f515da994f241b75d035e9c9e248dec2beeddc5b86d1c1069d94b2"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/feedback_limits_integration_test.go#L11-26"
    excerpt_sha256: "sha256:631f403c2acfd51a9f785b1fe5454d79db40e3ee8e961ba00572bdf444906a49"
---
The feedback message limit counts characters, not bytes, so 2000 Chinese characters are accepted.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A 2000-character Chinese message is accepted; 2001 are refused.
