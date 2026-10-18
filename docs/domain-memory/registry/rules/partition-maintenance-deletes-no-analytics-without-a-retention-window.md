---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:10:47.982008Z"
sources:
  - resource: "apps/platform/internal/product/learning/partitions.go#L26-33"
    excerpt_sha256: "sha256:990c849bcaa927329ecd2829d32c19399066342c195f8baec3f5b72bc988bf69"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/analytics_retention_integration_test.go#L53-84"
    excerpt_sha256: "sha256:25e6986fbea75a598489770f3f3e63927179450807a5ac46f151f2050ea2463c"
---
Partition maintenance without a positive retention window is refused and deletes no analytics events, since a zero window would delete everything.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- Maintenance with a zero window reports an error and leaves every event.
