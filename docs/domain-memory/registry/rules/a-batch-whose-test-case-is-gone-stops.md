---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L408-438"
    excerpt_sha256: "sha256:65c59c22f7d7a9b26eac1b275c201f12a679a063856768fc72eda07bcceea0e5"
  - resource: "apps/platform/internal/trial/batch/batch.go#L272-283"
    excerpt_sha256: "sha256:78953e71e4115a816caa77617f8239e8be9bd1a7f74925359ff1e35ab2908677"
---
When starting a batch's next trial finds its target gone, the batch stops with reason target_gone and starts no further trial.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A test case deleted mid-batch: the batch becomes stopped with target_gone.
