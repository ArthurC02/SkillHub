---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/http.go#L241-264"
    excerpt_sha256: "sha256:63c687d608ed5449b3cce49f5b72b2bbb4cca50e6e5913d5757573b2ccba5ccb"
---
An operator route lets a request through only for a user on the operator roster; anyone else gets 404 and a refused signed-in user is audited.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A signed-in non-operator asking for /admin/dispatch gets 404 and one operator-refused audit row.
