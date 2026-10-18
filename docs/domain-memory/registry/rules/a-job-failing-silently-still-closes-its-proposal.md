---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/operations/proposals.go#L231-238"
    excerpt_sha256: "sha256:df09f74be9edddff5127675954f11c26e231b676f900df19f6f9b9deeb657a93"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agent_proposals_integration_test.go#L464-476"
    excerpt_sha256: "sha256:d7423006eb5f9e0bfa919bc9a00e6a0a6a70edf0bd2f5caedfa381b969466213"
---
A maintenance job that fails with a blank message closes its proposal as failed with an outcome saying it failed without saying why.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A job error of '' closes the proposal as failed.
