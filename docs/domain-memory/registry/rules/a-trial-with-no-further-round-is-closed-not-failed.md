---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch_simulated_user.go#L121-123"
    excerpt_sha256: "sha256:3303ef97e8f0a6ca334b691864f220c7ba3b46a28b1702a9a2af7ba96cce8ba7"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_simulated_user_integration_test.go#L137-164"
    excerpt_sha256: "sha256:2662404ad1de5b6e9e3ae5cd0067ca25eb349285894f5d6cf74d5a6fb21a53d0"
---
A simulated answer refused because the conversation has no further round closes the trial instead of failing the batch.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A batch settles once no trial runs and no simulated answer is owed.
