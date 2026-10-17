---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/operations/findings.go#L149-155"
    excerpt_sha256: "sha256:3c9d463a383b8581258e7487a137f68103046907f373328e4c240550a5d1acdd"
---
After a successful report, every open or acknowledged finding the report did not cite becomes recovered, and reopens if a later report cites it.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A stuck-queue finding absent from today's report becomes recovered.
