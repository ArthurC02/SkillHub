---
type: Interaction
status: stable
consistency: "synchronous request, idempotent by origin key; the batch records the trial in its own statement afterwards"
delivery: composition-root port
verified:
  - by: "human:ArthurC"
    at: "2026-10-14T17:33:50.215879Z"
sources:
  - resource: "apps/platform/internal/entrypoint/wiring/evaluation_batches.go#L46-88"
    excerpt_sha256: "sha256:81f47701455768a93e0f9330943c273785dd34ee7ef9af66468493c2ee38f32d"
  - resource: "apps/platform/internal/trial/batch/batch.go#L241-276"
    excerpt_sha256: "sha256:b996b9107690f1352771454d2f1e03e35bef75a5ac6a27e3d1e7eb269da2ac32"
---
# Relations
- Producer context: [run](/contexts/run)
- Consumer context: [batch](/contexts/batch)
- Contract id: [run-batch-trials-v1](/contracts/run-batch-trials-v1)
