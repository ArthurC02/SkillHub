---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/design/http.go#L202-213"
    excerpt_sha256: "sha256:28fd7728217de69a3375baddcc20db26669355ff90a9d0a460c5744ca5beb46b"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/testlab_integration_test.go#L978-1007"
    excerpt_sha256: "sha256:ca207f418343ffd737e30a2af27a10f54ad7927f3e0d20cdcd7438309093e830"
---
The test case list takes a limit from 1 to 101 inclusive and defaults to 51, one row more than a page shows so the client can tell whether another page exists; 0, 102, a non-integer and an empty limit are refused, never replaced by the default.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- limit=101 lists; limit=102 and limit= answer 400; no limit lists 51.
