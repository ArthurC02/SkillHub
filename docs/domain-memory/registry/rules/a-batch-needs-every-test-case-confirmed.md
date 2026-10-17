---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L191-208"
    excerpt_sha256: "sha256:ca8f118919d1dc686914a1561d46212438c88e538e9e2b789eb75c4a6ab1b11b"
---
A batch is refused as a whole, naming every unconfirmed test case, when any of its test cases lacks confirmed run permissions.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- Two test cases, one unconfirmed: 422 naming only that one, and nothing is saved.
