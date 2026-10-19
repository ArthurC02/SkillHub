---
type: Interaction
status: stable
consistency: "synchronous request, idempotent by origin key; the batch records the trial in its own statement afterwards"
delivery: composition-root port
verified:
  - by: "human:ArthurC"
    at: "2026-10-19T17:32:50.132592Z"
sources:
  - resource: "apps/platform/internal/entrypoint/wiring/evaluation_batches.go#L46-88"
    excerpt_sha256: "sha256:81f47701455768a93e0f9330943c273785dd34ee7ef9af66468493c2ee38f32d"
  - resource: "apps/platform/internal/trial/batch/batch.go#L253-291"
    excerpt_sha256: "sha256:f0c2583a4f83f27d85d6d02b901c673b3c97229b78aef28f391e2216a5949da5"
---
# Relations
- Producer context: [run](/contexts/run)
- Consumer context: [batch](/contexts/batch)
- Contract id: [run-batch-trials-v1](/contracts/run-batch-trials-v1)
