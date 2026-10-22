---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0123_evaluation_batch_pause.sql#L1-7"
    excerpt_sha256: "sha256:436b7fb6394f8a2a09f3cd4ea160a9ae7e2447884e9328b5eb3a921d4ef3cb4d"
---
evaluation_batches keeps check evaluation_batches_stop_has_reason: CONSTRAINT evaluation_batches_stop_has_reason CHECK ((status IN ('paused', 'stopped')) = (stop_reason IS NOT NULL))

# Relations
- Contexts: [batch](/contexts/batch)
