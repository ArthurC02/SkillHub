---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:44:34.068147Z"
sources:
  - resource: "apps/platform/internal/product/learning/analytics.go#L41-43"
    excerpt_sha256: "sha256:7ff5a7e8aa2563338a6170d0a81cc54c6f24a8511be5ce7afc2a961926568057"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/beta_integration_test.go#L508-529"
    excerpt_sha256: "sha256:6a312f899b359297d14fdc2922827e0be2d94c9ca2e8b57f31fc80026755af14"
---
No funnel event is collected and no analytics cookie is set unless a retention period of at least one second is configured.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A deployment with no retention period records no search or detail event.
