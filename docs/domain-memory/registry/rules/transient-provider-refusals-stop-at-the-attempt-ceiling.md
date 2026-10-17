---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T18:18:26.030858Z"
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L240-262"
    excerpt_sha256: "sha256:65c482960460daa739b2ca93247051f448365848a6274d0e6585e3cce6767ed6"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/run_orchestration_integration_test.go#L545-564"
    excerpt_sha256: "sha256:d44f4a05fe8d9c2478b0ec053382129c416d97a7a0063235b241be59761db435"
---
Transient provider refusals start at most the configured number of attempts, after which the run fails as a provider error.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- With two attempts allowed and the provider answering 503 every time, the run ends failed with two attempts.
