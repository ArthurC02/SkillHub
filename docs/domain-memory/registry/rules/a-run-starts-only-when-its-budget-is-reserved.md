---
type: Business Rule
status: stable
owner: run-execution
version: unknown
effective_from: unknown
effective_until: unknown
verified:
  - by: "human:ArthurC"
    at: "2026-10-23T03:56:02.779143Z"
sources:
  - resource: "apps/platform/internal/trial/execution/credit.go#L18-32"
    excerpt_sha256: "sha256:b03908f4fa9e0d491325f7655607d7471852a8a7807f8805a05d178ba5d0b874"
  - resource: "apps/platform/internal/trial/execution/service.go#L484-488"
    excerpt_sha256: "sha256:8a51b03b8c4724e338df9afdc667fa4caa5b8645c5383b09a892ac2661f15061"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/run_budget_setting_integration_test.go#L75-124"
    excerpt_sha256: "sha256:e70def8a7eb0f315a27a4036d0054d674b861330b5d7e50e1c5ce2e4abb73dfa"
---
When a credit ledger is configured, a Run is admitted only if the Workspace can reserve the run budget in force when it is created: the operator-set ceiling, or the deployment's when none is set. The Run keeps that ceiling in its policy snapshot, and its attempts run under it whatever the setting becomes later; a Workspace that cannot reserve it is refused with credit_balance.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A Workspace with 100 credits is refused with credit_balance under a 0.50 USD ceiling and admitted once the operator lowers it to 0.05 USD.
- A Run created under a 0.05 USD ceiling issues its first gateway key at 0.05 USD after the operator raises the ceiling to 2 USD.
- A deployment with no credit ledger admits the Run without a reservation.
