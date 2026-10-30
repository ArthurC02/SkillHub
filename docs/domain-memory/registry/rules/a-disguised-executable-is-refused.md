---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-30T13:49:36.146814Z"
sources:
  - resource: "apps/platform/internal/trial/design/filetype.go#L62-66"
    excerpt_sha256: "sha256:84d3d98cf34fb4651ab5f57f2772f4aed44f5706dcfb608d06a10e935b02246f"
  - resource: "apps/platform/internal/trial/design/filetype_test.go#L56-75"
    excerpt_sha256: "sha256:06fe609c44ac9d636307989282f4ad591018c4aa327e9c742cb7660cec2608d8"
---
An upload whose leading bytes match an executable or archive signature is refused as an unsupported type, whatever its name.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- data.csv beginning with the ELF magic is refused.
