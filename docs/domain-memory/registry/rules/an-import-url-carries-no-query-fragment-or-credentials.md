---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/admission/source_url.go#L14-22"
    excerpt_sha256: "sha256:e1c8105fa93164492bbdeb4daa39fc2151fb5ea1a476e911e408ffef333d4c7a"
---
An import URL holding a query, a fragment or credentials is refused before any request, without echoing them.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- https://github.com/o/r#secret is refused and the message does not contain secret.
