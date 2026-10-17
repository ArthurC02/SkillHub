---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:47:28.319050Z"
sources:
  - resource: "apps/platform/internal/trial/design/dataset.go#L76-78"
    excerpt_sha256: "sha256:c15f426d0c807786500f7719fd88f62f97411f5f43c9a464f1cdac40bc8f9f3c"
  - resource: "apps/platform/internal/trial/design/dataset_characterization_test.go#L101-122"
    excerpt_sha256: "sha256:c409da511c4d0da84b9cf5626452aa5b510b0193c3b9f3ba989952ba405de3cb"
---
A dataset file larger than MaxFileBytes is refused before anything is stored; empty and nameless files are refused too.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A file of 25 MB and one byte is refused.
