---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/dataset.go#L334-349"
    excerpt_sha256: "sha256:a7ea17c49ccdd871b48eb4c57b678a7f69a5b9c431b18987134bc9530de172ff"
---
A dataset can be deleted only through the Test Case that owns it; naming another Test Case finds nothing and removes nothing.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- Deleting Test Case B's dataset under Test Case A answers not found and B's file stays.
