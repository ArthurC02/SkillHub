---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L259-267"
    excerpt_sha256: "sha256:e238eec0c0131176e4ec23b7886a601f24f453431a717cc5e2a2f1c04bfc959f"
---
When starting a batch's next trial finds its target gone, the batch stops with reason target_gone and starts no further trial.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A test case deleted mid-batch: the batch becomes stopped with target_gone.
