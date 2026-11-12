---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-12T11:53:00.622260Z"
sources:
  - resource: "apps/platform/internal/trial/design/http.go#L235-239"
    excerpt_sha256: "sha256:27a766bd7e4dc94ed8278f4019d3a7abed9a043f889f78963c2209f5d360ddab"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/testlab_integration_test.go#L1019-1060"
    excerpt_sha256: "sha256:126d784c66c5c29b9201874ee687687da98abc2ecd49e00f049bbd5b00f1ee2b"
---
The test case list takes an offset from 0 to 2147483647 as a plain decimal integer; a negative, fractional, hexadecimal, empty or larger value is refused rather than read as 0.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- offset=2147483647 lists; offset=2147483648, offset=0x10 and offset=-1 answer 400.
