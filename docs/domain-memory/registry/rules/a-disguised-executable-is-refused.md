---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/filetype.go#L51-55"
    excerpt_sha256: "sha256:4608079bf33bfb95f582fb4e4d0ce9b9ebae7f49441acbd8caf379bf0869c8a4"
---
An upload whose leading bytes match an executable or archive signature is refused as an unsupported type, whatever its name.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- data.csv beginning with the ELF magic is refused.
