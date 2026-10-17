---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/learning/analytics.go#L109-119"
    excerpt_sha256: "sha256:419787f33c04c7f166758e4205e739551997ba88fabc90760e2f06fd48abf37a"
---
The daily visit cookie lasts until UTC midnight but never longer than the analytics retention period.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- With one hour of retention at noon UTC, the visit cookie lives one hour, not twelve.
