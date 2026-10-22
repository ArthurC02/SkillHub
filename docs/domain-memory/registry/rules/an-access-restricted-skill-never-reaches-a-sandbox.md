---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T16:46:22.676701Z"
sources:
  - resource: "apps/platform/internal/trial/execution/service.go#L490-514"
    excerpt_sha256: "sha256:d5b3ce56d56a3fa7ef8abf7dace064656c3b1d9d7dce6d974e5ebaef5bfad9de"
  - resource: "apps/platform/internal/trial/execution/job.go#L174-179"
    excerpt_sha256: "sha256:5fda0f513f4ab3ca1f69f66999132c2bea3e4167bc6823a4b6545c6821d198c5"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/preflight_integration_test.go#L329-369"
    excerpt_sha256: "sha256:99269089bfff4f8e108fb63c47cedaa14c6d45f61f0df459bd2fc489c6bb6d18"
---
A run of a skill under an access restriction is refused when it is created, and a run that was queued before the restriction was set fails as policy_refused at dispatch, before any sandbox receives the package; while the restriction cannot be read the run stays queued.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A run queued before an operator sets license-review fails as policy_refused naming the license review, and the sandbox sees no dispatch; after the restriction is cleared a new run succeeds.
