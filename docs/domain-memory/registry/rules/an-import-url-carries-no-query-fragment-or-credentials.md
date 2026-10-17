---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:08:38.278632Z"
sources:
  - resource: "apps/platform/internal/skill/admission/source_url.go#L14-22"
    excerpt_sha256: "sha256:e1c8105fa93164492bbdeb4daa39fc2151fb5ea1a476e911e408ffef333d4c7a"
  - resource: "apps/platform/internal/skill/admission/fetch_test.go#L32-53"
    excerpt_sha256: "sha256:b23102f5408fcc24a4c503a5684421bf26c8af9f70437d6f3b7ae29e3d2a5fc4"
---
An import URL holding a query, a fragment or credentials is refused before any request, without echoing them.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- https://github.com/o/r#secret is refused and the message does not contain secret.
