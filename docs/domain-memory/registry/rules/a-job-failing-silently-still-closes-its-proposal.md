---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-12T09:34:36.964247Z"
sources:
  - resource: "apps/platform/internal/product/operations/proposals.go#L231-238"
    excerpt_sha256: "sha256:df09f74be9edddff5127675954f11c26e231b676f900df19f6f9b9deeb657a93"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agent_proposals_integration_test.go#L464-476"
    excerpt_sha256: "sha256:0cd96fcda573475dbc718944126ae8c37618c9d4d17ffb62f9ccb21b1f0ef412"
---
A maintenance job that fails with a blank message closes its proposal as failed with an outcome saying it failed without saying why.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A job error of '' closes the proposal as failed.
