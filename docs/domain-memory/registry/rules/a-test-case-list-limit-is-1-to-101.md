---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-08T08:46:39.695261Z"
sources:
  - resource: "apps/platform/internal/trial/design/http.go#L235-242"
    excerpt_sha256: "sha256:5cdbcdceef5894e8975c4295cfacba5019ea27c785c5bc04dac5ef7f58bf3d87"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/testlab_integration_test.go#L978-1007"
    excerpt_sha256: "sha256:ca207f418343ffd737e30a2af27a10f54ad7927f3e0d20cdcd7438309093e830"
---
The test case list takes a limit from 1 to 101 inclusive and defaults to 51, one row more than a page shows so the client can tell whether another page exists; 0, 102, a non-integer and an empty limit are refused, never replaced by the default.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- limit=101 lists; limit=102 and limit= answer 400; no limit lists 51.
