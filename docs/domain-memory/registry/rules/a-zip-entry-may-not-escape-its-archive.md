---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/filetype.go#L102-104"
    excerpt_sha256: "sha256:bc543ac9f019dbe9236e51fcde206dc948af782d135af55374a8e113b6036cf5"
---
A zip upload is refused when any entry would escape the archive root by traversal or an absolute path.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A zip holding ../escape.txt is refused.
