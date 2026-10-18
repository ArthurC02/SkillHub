---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L151-156"
    excerpt_sha256: "sha256:87f36fb780867641c54c1b081537c1668215a633b7de8953b4e0d09a8ab2cb93"
  - resource: "apps/platform/internal/product/learning/feedback_characterization_test.go#L30-47"
    excerpt_sha256: "sha256:cf451702b84ee8d905119f0f1fda430f335dcc316d0e395f244b4fbf01fdbbb0"
---
A reported build id is kept only when it is non-blank and at most 64 bytes; otherwise it is dropped and the report is still accepted.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A 65-character build id is stored as absent; a 64-character one is kept.
