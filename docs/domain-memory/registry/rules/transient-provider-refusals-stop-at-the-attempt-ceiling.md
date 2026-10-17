---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L240-262"
    excerpt_sha256: "sha256:65c482960460daa739b2ca93247051f448365848a6274d0e6585e3cce6767ed6"
---
Transient provider refusals start at most the configured number of attempts, after which the run fails as a provider error.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- With two attempts allowed and the provider answering 503 every time, the run ends failed with two attempts.
