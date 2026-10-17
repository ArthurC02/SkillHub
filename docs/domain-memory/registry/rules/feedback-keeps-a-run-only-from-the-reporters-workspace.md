---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:44:34.068147Z"
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L124-134"
    excerpt_sha256: "sha256:17ee6ee2434e519fb11b4c221395a44dc6de9d1265736f346ce81cdc30e8a159"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/beta_integration_test.go#L622-659"
    excerpt_sha256: "sha256:01f78e901941335ead5745e6fcfa5a17ce5fbb885d4c682981337c45459a586e"
---
A run named on a feedback report is kept only when it belongs to the reporter's Workspace; another Workspace's run is dropped and the report is still accepted.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- Alice reports with Bob's run id: the report is filed with no run.
