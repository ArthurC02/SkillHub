---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/admission/fetch.go#L231-241"
    excerpt_sha256: "sha256:2a04fbc39b6e47f21e35a6d382e55c3cddfc6b118e7ccd2c2835e2000a2b9e6b"
  - resource: "apps/platform/internal/skill/admission/fetch_test.go#L104-123"
    excerpt_sha256: "sha256:a7401075fbc4fab5abd08c775320a3eb4e7badf6bc863097c0cc7196654b0d3c"
---
An import from a URL accepts a package of exactly the zip ceiling and refuses one byte more.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A server answering with one byte over the ceiling is refused naming the 10.0 MB limit.
