---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:13:02.301694Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L251-253"
    excerpt_sha256: "sha256:ffb91cb46bacd98cfd80a1f3fb041bbfe19400dec91a6b1d8c9db961752249f0"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L349-375"
    excerpt_sha256: "sha256:d040fb142634b949d1226750c92785df9643761a1eefe0105ae21cfa9ad336b7"
---
When a batch's creator no longer exists, the batch stops as creator_gone instead of starting another trial with no actor.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- The creator's account is removed mid-batch; the next advance stops the batch and starts nothing.
