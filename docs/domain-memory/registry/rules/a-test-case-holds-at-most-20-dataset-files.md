---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/dataset.go#L251-254"
    excerpt_sha256: "sha256:4cf6700bee0a55349f14a7c75c5f2829f7086892cb4f530ecd601546fdf4aa37"
---
A Test Case holds at most MaxFilesPerTestCase live dataset files; one more is refused and the object already written is removed.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- With 20 files on the case, a 21st upload is refused and its object removed.
