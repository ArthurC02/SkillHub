---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L160-184"
    excerpt_sha256: "sha256:8ef88772f32292ea1531fff76c56d0024661068f021cdcdd422189bac17bfe9b"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agent_proposals_integration_test.go#L710-739"
    excerpt_sha256: "sha256:364a963d8ed718922dca007e22d493f3ded277895e120bdedbe475abd5ad4c69"
---
Purging feedback removes only reports created before the retention window, and the preview counts exactly what the purge removes.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- With a 90-day window, reports aged 100 and 91 days are removed and a fresh one stays.
