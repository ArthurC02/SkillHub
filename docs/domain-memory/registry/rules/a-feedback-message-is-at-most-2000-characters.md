---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L109-113"
    excerpt_sha256: "sha256:c6aab9ace0f515da994f241b75d035e9c9e248dec2beeddc5b86d1c1069d94b2"
---
A feedback message is trimmed, must be non-blank and may hold at most 2000 characters; otherwise it is refused with 400.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A 2001-character message answers 400; exactly 2000 is accepted.
