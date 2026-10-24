---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L111-121"
    excerpt_sha256: "sha256:17ee6ee2434e519fb11b4c221395a44dc6de9d1265736f346ce81cdc30e8a159"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/beta_integration_test.go#L654-691"
    excerpt_sha256: "sha256:01f78e901941335ead5745e6fcfa5a17ce5fbb885d4c682981337c45459a586e"
---
A run named on a feedback report is kept only when it belongs to the reporter's Workspace; another Workspace's run is dropped and the report is still accepted.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- Alice reports with Bob's run id: the report is filed with no run.
