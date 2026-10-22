---
type: Interaction
status: stable
consistency: "synchronous request, idempotent by origin key; the batch records the trial in its own statement afterwards"
delivery: composition-root port
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T10:07:44.566259Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L253-291"
    excerpt_sha256: "sha256:f59f6851992723f666c594a0f96028394f3305b6ef1ef897c7ca561cf9d36999"
  - resource: "apps/platform/internal/entrypoint/wiring/evaluation_batches.go#L46-91"
    excerpt_sha256: "sha256:9f717648182cc8b8d70ff7ea41570ebaaaf9a022a738b57c03e730ee343ce676"
---
# Relations
- Producer context: [run](/contexts/run)
- Consumer context: [batch](/contexts/batch)
- Contract id: [run-batch-trials-v1](/contracts/run-batch-trials-v1)
