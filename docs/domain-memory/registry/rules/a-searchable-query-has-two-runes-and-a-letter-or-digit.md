---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T22:59:26.747795Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/http.go#L569-589"
    excerpt_sha256: "sha256:061e19640a49ea2a04696078b802e29bcd4427aa74f11b7b7f22086999a69ebc"
  - resource: "apps/platform/internal/skill/discovery/reason_test.go#L151-182"
    excerpt_sha256: "sha256:7c4ab4eca906e0d257bbb8ea1f66b20d09d405469c6d2563a6dcc5d621c2920c"
---
A query is searched only when it is valid text without control characters, has at least two characters and holds a letter or digit.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- '你' and '!!' are not searched; 'C#' and '轉表格' are.
