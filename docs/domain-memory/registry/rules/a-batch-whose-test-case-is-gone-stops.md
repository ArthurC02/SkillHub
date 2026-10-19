---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-19T17:32:50.132592Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L271-282"
    excerpt_sha256: "sha256:64758ed135fd11623975e273461b4358defd67b4b3f2c70a6caa59e7be605df4"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L317-347"
    excerpt_sha256: "sha256:65c59c22f7d7a9b26eac1b275c201f12a679a063856768fc72eda07bcceea0e5"
---
When starting a batch's next trial finds its target gone, the batch stops with reason target_gone and starts no further trial.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A test case deleted mid-batch: the batch becomes stopped with target_gone.
