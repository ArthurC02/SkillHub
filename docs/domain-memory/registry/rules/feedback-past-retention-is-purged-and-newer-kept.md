---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:10:47.982008Z"
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L184-193"
    excerpt_sha256: "sha256:400e15fff87bba67e02938629c2bd4c61bde74dce82e6791a217e0a1a1d895bf"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agent_proposals_integration_test.go#L710-739"
    excerpt_sha256: "sha256:364a963d8ed718922dca007e22d493f3ded277895e120bdedbe475abd5ad4c69"
---
Purging feedback removes only reports created before the retention window, and the preview counts exactly what the purge removes.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- With a 90-day window, reports aged 100 and 91 days are removed and a fresh one stays.
