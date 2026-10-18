---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:10:47.982008Z"
sources:
  - resource: "apps/platform/internal/product/learning/analytics.go#L185-206"
    excerpt_sha256: "sha256:b282bc4324733cf13bc0628b8aab795909ee26f7c9cefaf90046bf6dc29f0442"
  - resource: "apps/platform/internal/product/learning/analytics_test.go#L38-53"
    excerpt_sha256: "sha256:23a32cbf48b88c97eaf7442a6ae99070728285d77526ca6580108fa7e256b539"
---
A search query is recorded only by its script bucket, with Han, Hiragana, Katakana and Hangul all bucketed as han; the text itself is never stored.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A Hangul query is bucketed han.
