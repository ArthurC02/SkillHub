---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/design/http.go#L215-225"
    excerpt_sha256: "sha256:d1259489d1e4fe69e31e3e6ef92ec650dd749ca6ec1910aabd8dd4c79c70e8a7"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/testlab_integration_test.go#L1019-1060"
    excerpt_sha256: "sha256:126d784c66c5c29b9201874ee687687da98abc2ecd49e00f049bbd5b00f1ee2b"
---
The test case list takes an offset from 0 to 2147483647 as a plain decimal integer; a negative, fractional, hexadecimal, empty or larger value is refused rather than read as 0.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- offset=2147483647 lists; offset=2147483648, offset=0x10 and offset=-1 answer 400.
