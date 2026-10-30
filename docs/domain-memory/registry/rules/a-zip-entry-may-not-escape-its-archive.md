---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-30T13:49:36.146814Z"
sources:
  - resource: "apps/platform/internal/trial/design/filetype.go#L113-115"
    excerpt_sha256: "sha256:985ba27d331b1636f5578d4500640d23b02e655549eebfbf261182bd07eb35d0"
  - resource: "apps/platform/internal/trial/design/filetype_test.go#L106-121"
    excerpt_sha256: "sha256:0f1417be9843b13d8f9b609f83bda4e55d3cc579224fdb822dba5d951d186766"
---
A zip upload is refused when any entry would escape the archive root by traversal or an absolute path.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A zip holding ../escape.txt is refused.
