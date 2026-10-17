---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/admission/fetch.go#L233-243"
    excerpt_sha256: "sha256:2a04fbc39b6e47f21e35a6d382e55c3cddfc6b118e7ccd2c2835e2000a2b9e6b"
---
An import from a URL accepts a package of exactly the zip ceiling and refuses one byte more.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A server answering with one byte over the ceiling is refused naming the 10.0 MB limit.
