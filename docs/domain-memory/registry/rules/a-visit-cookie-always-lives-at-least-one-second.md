---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T14:00:33.042847Z"
sources:
  - resource: "apps/platform/internal/product/learning/analytics.go#L121-123"
    excerpt_sha256: "sha256:f7a007be9a86ea7e3202cc9b5fe452c536e43a4bff95d84ca30b71c089ab419b"
  - resource: "apps/platform/internal/product/learning/analytics_test.go#L156-172"
    excerpt_sha256: "sha256:97d9283ddc29258846c707c1f5f9f1400ae61db076ced7bc8b33e23cb83dcc1d"
---
A visit cookie's lifetime never drops below one second, so in the last second before UTC midnight it still expires instead of becoming a browser-session cookie.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- At 23:59:59.5 UTC the visit cookie gets Max-Age 1, not 0.
