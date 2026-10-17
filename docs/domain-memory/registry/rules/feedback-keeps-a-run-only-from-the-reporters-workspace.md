---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L124-134"
    excerpt_sha256: "sha256:17ee6ee2434e519fb11b4c221395a44dc6de9d1265736f346ce81cdc30e8a159"
---
A run named on a feedback report is kept only when it belongs to the reporter's Workspace; another Workspace's run is dropped and the report is still accepted.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- Alice reports with Bob's run id: the report is filed with no run.
