---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:13:02.301694Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L259-267"
    excerpt_sha256: "sha256:e238eec0c0131176e4ec23b7886a601f24f453431a717cc5e2a2f1c04bfc959f"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L317-347"
    excerpt_sha256: "sha256:65c59c22f7d7a9b26eac1b275c201f12a679a063856768fc72eda07bcceea0e5"
---
When starting a batch's next trial finds its target gone, the batch stops with reason target_gone and starts no further trial.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A test case deleted mid-batch: the batch becomes stopped with target_gone.
