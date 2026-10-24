---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L654-661"
    excerpt_sha256: "sha256:29c51a42469697687c591ff20c6b2dcfb973b8271c8414d4c03f3942b391c866"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/exposure_integration_test.go#L484-514"
    excerpt_sha256: "sha256:f98c7487952d1d5a1e9026e78f90405fb6f0e85d1ef619b7844a65ccffed5b43"
---
Publishing the published latest version again, with the same content and the same declared licence, adds no release, so its exposure review is kept; declaring a different licence for that version adds a release.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Publishing v1 twice leaves exactly one release; publishing it again with MIT adds a second, and once more with MIT still leaves two.
