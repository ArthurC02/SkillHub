---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T22:48:52.366604Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/consumer.go#L64-108"
    excerpt_sha256: "sha256:33488562383e1078444b1794af7e88cb16108f61e769fdda74729aca78b62caf"
  - resource: "db/queries/runs.sql#L394-401"
    excerpt_sha256: "sha256:dbd7b2941986772cd727383424957612fd207c197585d76a739215aef7a2e795"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/unevaluated_run_sweep_integration_test.go#L35-75"
    excerpt_sha256: "sha256:f763701ea710e0f644c556b40ebd566d5714bdc41f2f6cd2179d8878231f87be"
---
Every run that finished as succeeded or failed is evaluated even when the event or job that triggers its evaluation exhausted its retries: every 30 minutes, runs that finished more than 30 minutes and less than a day ago and have no evaluation record are enqueued for evaluation again, through the same enqueue that ignores a run whose evaluation job is still live.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A succeeded run whose run-succeeded event was dead-lettered has no evaluation; the next sweep enqueues it. A cancelled run, a run that finished ten minutes ago, a run that finished two days ago and a run that already has an evaluation are left alone.
