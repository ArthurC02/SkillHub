---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L253-258"
    excerpt_sha256: "sha256:2ed17076347abb93b8232b5a2b059e7a31cf7fbbc274594f43ce620c85494987"
  - resource: "apps/platform/internal/skill/publishing/service.go#L637-654"
    excerpt_sha256: "sha256:4f542c48510815605afd328c19586613d7d0669e59b80b98b00b03833ae6e8ca"
---
Publishing the published latest version again, with the same content, adds no release, so its exposure review is kept.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Publishing v1 twice leaves exactly one release.
