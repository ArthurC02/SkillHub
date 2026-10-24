---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L224-224"
    excerpt_sha256: "sha256:b8f35b6c48fb87022e7d451eb0e07f81c3aff752dd949399b59dd0b39b334fe4"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L377-406"
    excerpt_sha256: "sha256:cd0fbf89b116ca8aa503a9d8e4aa6c21678d873756f18e35f30edb1c98260a48"
---
A batch paused for 30 days stops with reason paused_too_long; one paused for less stays paused and can be resumed from its next unrun trial.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- Paused 30 days and a minute stops; 29 days 23 hours stays paused.
