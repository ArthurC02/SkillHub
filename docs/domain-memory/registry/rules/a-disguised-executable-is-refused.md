---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:47:28.319050Z"
sources:
  - resource: "apps/platform/internal/trial/design/filetype.go#L51-55"
    excerpt_sha256: "sha256:4608079bf33bfb95f582fb4e4d0ce9b9ebae7f49441acbd8caf379bf0869c8a4"
  - resource: "apps/platform/internal/trial/design/filetype_test.go#L56-75"
    excerpt_sha256: "sha256:06fe609c44ac9d636307989282f4ad591018c4aa327e9c742cb7660cec2608d8"
---
An upload whose leading bytes match an executable or archive signature is refused as an unsupported type, whatever its name.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- data.csv beginning with the ELF magic is refused.
