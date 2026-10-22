---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T10:07:44.566259Z"
sources:
  - resource: "apps/platform/internal/entrypoint/wiring/evaluation_batches.go#L48-51"
    excerpt_sha256: "sha256:91f030e8fd5cae4aefcf7c34c2a1aff83d9f7354b6ae6c51bf9d7c17d9924180"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/p1_halt_model_calls_integration_test.go#L107-141"
    excerpt_sha256: "sha256:fa35d8e841fc93b0d5d35f498206c78c12df64653641b69be9758fab92901c4e"
---
While dispatch is paused for every provider, by an orphan threshold or a P1 incident, an evaluation batch waits instead of creating its next trial, and starts it on the first advance after the halt lifts.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A batch created under an orphan-threshold pool halt has no trial run; after the halt lifts the next advance starts its one trial.
