---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-01T13:59:08.847162Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/reason_test.go#L151-187"
    excerpt_sha256: "sha256:053f4247fae9701866ec6edf169dafc459aadee5d627db25050177e72809c5b1"
  - resource: "apps/platform/internal/skill/discovery/http.go#L569-590"
    excerpt_sha256: "sha256:28609d9979c6ec28bcf583f46a826ee311c21a534a3a8534857ca222e58f3b5a"
---
A query is searched only when it is valid text without control characters and holds a letter or digit, and it has at least two characters unless it is one Han character.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- 'R', '1', 'ア' and '!!' are not searched; '表', '𠮟', 'C#' and '轉表格' are.
