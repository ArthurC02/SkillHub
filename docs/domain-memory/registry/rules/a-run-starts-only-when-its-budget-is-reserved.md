---
type: Business Rule
status: stable
owner: run-execution
version: unknown
effective_from: unknown
effective_until: unknown
verified:
  - by: "human:ArthurC"
    at: "2026-10-23T05:42:31.923187Z"
sources:
  - resource: "apps/platform/internal/trial/execution/credit.go#L18-32"
    excerpt_sha256: "sha256:b03908f4fa9e0d491325f7655607d7471852a8a7807f8805a05d178ba5d0b874"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/run_budget_setting_integration_test.go#L85-134"
    excerpt_sha256: "sha256:d39f0b17e410dcf9b0e75848cb089d3a790361b89f957f159b72f00f1483ac73"
  - resource: "apps/platform/internal/trial/execution/service.go#L486-490"
    excerpt_sha256: "sha256:8a51b03b8c4724e338df9afdc667fa4caa5b8645c5383b09a892ac2661f15061"
---
When a credit ledger is configured, a Run is admitted only if the Workspace can reserve the run budget in force when it is created: the operator-set ceiling in run_model_budget_setting, or the deployment's when none is set. The Run keeps that ceiling in its policy snapshot, and its attempts run under it whatever the setting becomes later; a Workspace that cannot reserve it is refused with credit_balance.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A Workspace with 100 credits is refused with credit_balance under a 0.50 USD ceiling and admitted once the operator lowers it to 0.05 USD.
- A Run created under a 0.05 USD ceiling issues its first gateway key at 0.05 USD after the operator raises the ceiling to 2 USD.
- A deployment with no credit ledger admits the Run without a reservation.
