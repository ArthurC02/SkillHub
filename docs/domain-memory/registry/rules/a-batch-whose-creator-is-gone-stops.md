---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L251-253"
    excerpt_sha256: "sha256:ffb91cb46bacd98cfd80a1f3fb041bbfe19400dec91a6b1d8c9db961752249f0"
---
When a batch's creator no longer exists, the batch stops as creator_gone instead of starting another trial with no actor.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- The creator's account is removed mid-batch; the next advance stops the batch and starts nothing.
