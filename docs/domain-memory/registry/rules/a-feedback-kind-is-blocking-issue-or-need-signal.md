---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L105-108"
    excerpt_sha256: "sha256:70eb94f8e916828cc8ff788cd220269fc1405ece7f254163469e149ecc20fa0d"
---
A feedback kind must be blocking_issue or need_signal; any other or a missing kind is refused with 400.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- kind praise answers 400.
