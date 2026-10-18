---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:10:47.982008Z"
sources:
  - resource: "apps/platform/internal/product/learning/partitions.go#L26-33"
    excerpt_sha256: "sha256:990c849bcaa927329ecd2829d32c19399066342c195f8baec3f5b72bc988bf69"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/analytics_retention_integration_test.go#L11-51"
    excerpt_sha256: "sha256:41080d03047fe15046f0d532b6041a4d6dc57addc79bfad19d26d9d4070e1ed3"
---
Maintenance deletes analytics events strictly older than the retention window; an event exactly at the cutoff is kept.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- With a ten-day window, an event one second before the cutoff is deleted and one at the cutoff is kept.
