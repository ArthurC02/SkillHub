---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:10:47.982008Z"
sources:
  - resource: "apps/platform/internal/product/learning/analytics.go#L109-112"
    excerpt_sha256: "sha256:9783ded5fd7362c53430a70403df577aca4256d4e02a8fdb86b62e529a8fa9b5"
  - resource: "apps/platform/internal/product/learning/analytics_test.go#L13-26"
    excerpt_sha256: "sha256:b5701d783edd736e4f91e0cdc678d79ff30f5c670a18871460668f880a7e16d5"
---
A visit starts, and its cookie is reissued, only when the request has no visit cookie or one dated before today in UTC.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A second request the same UTC day is not a new visit; the first after UTC midnight is.
