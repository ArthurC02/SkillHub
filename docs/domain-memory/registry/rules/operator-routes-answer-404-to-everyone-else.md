---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/http.go#L244-267"
    excerpt_sha256: "sha256:63c687d608ed5449b3cce49f5b72b2bbb4cca50e6e5913d5757573b2ccba5ccb"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/identity_hardening_a3_integration_test.go#L17-57"
    excerpt_sha256: "sha256:5b197623c20cb91614af3b18876842d267bece2dcaa036d12fe8ed5c2a9b7de9"
---
An operator route lets a request through only for a user on the operator roster; anyone else gets 404 and a refused signed-in user is audited.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A signed-in non-operator asking for /admin/dispatch gets 404 and one operator-refused audit row.
