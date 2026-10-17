---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:47:28.319050Z"
sources:
  - resource: "apps/platform/internal/trial/design/filetype.go#L102-104"
    excerpt_sha256: "sha256:bc543ac9f019dbe9236e51fcde206dc948af782d135af55374a8e113b6036cf5"
  - resource: "apps/platform/internal/trial/design/filetype_test.go#L106-121"
    excerpt_sha256: "sha256:0f1417be9843b13d8f9b609f83bda4e55d3cc579224fdb822dba5d951d186766"
---
A zip upload is refused when any entry would escape the archive root by traversal or an absolute path.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A zip holding ../escape.txt is refused.
