---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:47:28.319050Z"
sources:
  - resource: "apps/platform/internal/trial/design/dataset.go#L251-254"
    excerpt_sha256: "sha256:4cf6700bee0a55349f14a7c75c5f2829f7086892cb4f530ecd601546fdf4aa37"
  - resource: "apps/platform/internal/trial/design/dataset_ownership_integration_test.go#L415-435"
    excerpt_sha256: "sha256:2bc57a0f21e3c8eef4047ed2e863700cfb6ead69dc453f65ae3d3d43579e97ad"
---
A Test Case holds at most MaxFilesPerTestCase live dataset files; one more is refused and the object already written is removed.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- With 20 files on the case, a 21st upload is refused and its object removed.
