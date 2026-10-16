---
type: Business Rule
status: draft
owner: run-execution
version: unknown
effective_from: unknown
effective_until: unknown
sources:
  - resource: "apps/platform/internal/trial/execution/credit.go#L18-32"
    excerpt_sha256: "sha256:a9ec1408a833dd3ed97f0dff32201070c8faca55a24fdf7dfe0b28dc6705237d"
---
When a credit ledger is configured, a Run is admitted only if the Workspace can reserve the deployment's run budget; otherwise it is refused with credit_balance.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A Workspace whose balance is below the run budget is refused with credit_balance and no Run is created.
- A deployment with no credit ledger admits the Run without a reservation.
