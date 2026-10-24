---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L198-215"
    excerpt_sha256: "sha256:ca8f118919d1dc686914a1561d46212438c88e538e9e2b789eb75c4a6ab1b11b"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L64-103"
    excerpt_sha256: "sha256:e81314b24b08c5ee37b8d39ec9bd58f93f0d7734f3b01ed16175ca4224756eff"
---
A batch is refused as a whole, naming every unconfirmed test case, when any of its test cases lacks confirmed run permissions.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- Two test cases, one unconfirmed: 422 naming only that one, and nothing is saved.
