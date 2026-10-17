---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/snapshot.go#L249-258"
    excerpt_sha256: "sha256:c341a031d1d790d46c9c07526a0c76166ee436b812681f92561c7833edeb73f3"
---
A snapshot's content hash covers the prompt with the criteria, datasets, rubric and facts, so any edit freezes a new snapshot and earlier ones stay as they were.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- Snapshotting after changing the prompt gives a different hash and leaves the first snapshot's prompt unchanged.
