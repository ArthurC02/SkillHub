---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/dataset.go#L76-78"
    excerpt_sha256: "sha256:c15f426d0c807786500f7719fd88f62f97411f5f43c9a464f1cdac40bc8f9f3c"
---
A dataset file larger than MaxFileBytes is refused before anything is stored; empty and nameless files are refused too.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A file of 25 MB and one byte is refused.
