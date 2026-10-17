---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T18:18:26.030858Z"
sources:
  - resource: "apps/platform/internal/trial/execution/schedule.go#L564-569"
    excerpt_sha256: "sha256:55aaf0b38bc08527338b0dd3b23de3e89ef09a6ddf19cc597e703161a7df4f32"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/run_orchestration_integration_test.go#L566-581"
    excerpt_sha256: "sha256:1a6598a1a05d6065f75e05517b620ae5d5c023e2b0a02c21d2f31b9feb059291"
---
Every dispatch to a provider carries the platform's own attempt id as its idempotency key, so a re-sent dispatch is deduplicated per attempt and no provider id stands in for the platform's identity.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A run retried after a 503 dispatches its second attempt keyed by that attempt's id, not the run id.
