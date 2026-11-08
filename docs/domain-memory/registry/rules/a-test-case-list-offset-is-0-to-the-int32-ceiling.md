---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-08T08:46:39.695261Z"
sources:
  - resource: "apps/platform/internal/trial/design/http.go#L244-246"
    excerpt_sha256: "sha256:4dae8e8ed091367cee69394cbaa1483ecb4ec77da9c411f3ccc4182e191dc096"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/testlab_integration_test.go#L1019-1060"
    excerpt_sha256: "sha256:126d784c66c5c29b9201874ee687687da98abc2ecd49e00f049bbd5b00f1ee2b"
---
The test case list takes an offset from 0 to 2147483647 as a plain decimal integer; a negative, fractional, hexadecimal, empty or larger value is refused rather than read as 0.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- offset=2147483647 lists; offset=2147483648, offset=0x10 and offset=-1 answer 400.
