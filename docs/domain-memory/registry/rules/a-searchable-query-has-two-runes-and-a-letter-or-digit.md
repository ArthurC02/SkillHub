---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/discovery/http.go#L567-587"
    excerpt_sha256: "sha256:061e19640a49ea2a04696078b802e29bcd4427aa74f11b7b7f22086999a69ebc"
---
A query is searched only when it is valid text without control characters, has at least two characters and holds a letter or digit.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- '你' and '!!' are not searched; 'C#' and '轉表格' are.
