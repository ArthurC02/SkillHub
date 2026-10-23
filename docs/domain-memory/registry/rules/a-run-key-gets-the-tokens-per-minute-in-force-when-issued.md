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
  - resource: "apps/platform/internal/trial/execution/schedule.go#L512-526"
    excerpt_sha256: "sha256:7db70c2861a742c65279f29e957b4b5a5c1baefe9d394513e0e83fc1d4e4cf66"
  - resource: "apps/platform/internal/trial/execution/settings.go#L242-255"
    excerpt_sha256: "sha256:3e1c5c9cf48bed06ffbaf6b4371219d589b3e3934f433242dc3e221dd6d6c783"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/run_budget_setting_integration_test.go#L199-219"
    excerpt_sha256: "sha256:711332f125d91eb7b9e1664236e08a226db6289a7824e247cd871749efb87a6d"
---
Every model gateway key issued for a run attempt gets the tokens-per-minute limit in force when it is issued: the operator-set value in run_rate_limit_setting, or the deployment's when none is set. The limit is not kept in the Run's snapshot, so a change reaches the next attempt of a Run already created.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A Run created before the operator sets 20000 tokens per minute has its first attempt's key issued at 20000.
- With no operator-set value, a key is issued at the deployment's limit, 200000 by default.
