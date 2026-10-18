---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:10:47.982008Z"
sources:
  - resource: "apps/platform/internal/product/learning/policy.go#L10-18"
    excerpt_sha256: "sha256:4ff6c919d41d9b3fe48f2364bd739c7c7cd84ac93f3896e1e25b10fb44916a42"
  - resource: "apps/platform/internal/product/learning/policy_test.go#L88-102"
    excerpt_sha256: "sha256:bde829d456cd603857db5e37adb64e00bdb62d370c3ae1f32f5c2dfdd3cce64d"
---
The public retention disclosure reports whole days rounded up, so a window under a day reads as one day, never zero.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A twelve-hour window is disclosed as one day.
