---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/learning/analytics.go#L41-43"
    excerpt_sha256: "sha256:7ff5a7e8aa2563338a6170d0a81cc54c6f24a8511be5ce7afc2a961926568057"
---
No funnel event is collected and no analytics cookie is set unless a retention period of at least one second is configured.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A deployment with no retention period records no search or detail event.
