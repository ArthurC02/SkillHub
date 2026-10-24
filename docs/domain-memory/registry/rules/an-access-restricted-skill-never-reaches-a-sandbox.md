---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/execution/service.go#L506-530"
    excerpt_sha256: "sha256:93fc41c9e61caed47c19d53aba63834e2aea46dc488f83b259aa084349b48e04"
  - resource: "apps/platform/internal/trial/execution/job.go#L174-181"
    excerpt_sha256: "sha256:4810ff4a644c9d8dd79c248465077894aab424b2f122ba150a3d761898a5d2af"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/preflight_integration_test.go#L333-373"
    excerpt_sha256: "sha256:99269089bfff4f8e108fb63c47cedaa14c6d45f61f0df459bd2fc489c6bb6d18"
---
A run of a skill under an access restriction is refused when it is created, and a run that was queued before the restriction was set fails as policy_refused at dispatch, before any sandbox receives the package; while the restriction cannot be read the run stays queued.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A run queued before an operator sets license-review fails as policy_refused naming the license review, and the sandbox sees no dispatch; after the restriction is cleared a new run succeeds.
