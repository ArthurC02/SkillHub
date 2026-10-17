---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/admission/fetch.go#L106-114"
    excerpt_sha256: "sha256:10556d4399e7ec88b71d2b948ba025a53118e7a2d8568d296e34ef3e7049c97b"
---
An import from a URL follows at most three redirects and then stops.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A server that always redirects sees four requests before the import fails.
