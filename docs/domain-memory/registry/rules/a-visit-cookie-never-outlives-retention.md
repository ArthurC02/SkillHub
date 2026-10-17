---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:44:34.068147Z"
sources:
  - resource: "apps/platform/internal/product/learning/analytics.go#L109-119"
    excerpt_sha256: "sha256:419787f33c04c7f166758e4205e739551997ba88fabc90760e2f06fd48abf37a"
  - resource: "apps/platform/internal/product/learning/analytics_test.go#L28-36"
    excerpt_sha256: "sha256:41489261ce19608ded685ac978707e89b3301a89593d6a4032edc2848e7a25da"
---
The daily visit cookie lasts until UTC midnight but never longer than the analytics retention period.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- With one hour of retention at noon UTC, the visit cookie lives one hour, not twelve.
