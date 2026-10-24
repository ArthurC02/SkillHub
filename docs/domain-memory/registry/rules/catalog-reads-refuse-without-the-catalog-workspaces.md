---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T22:59:26.747795Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/service.go#L349-354"
    excerpt_sha256: "sha256:e687c0440aad31ffb1812aac767849d8788d323c8f79d11733841fa0f4f97df2"
  - resource: "apps/platform/internal/skill/discovery/scope_test.go#L10-22"
    excerpt_sha256: "sha256:d55e20ac041d24c5f32e37e299cd3ffba0b70653a7f95af5fa880a0e3cf41c2b"
---
Browsing, Catalog risk reads and reference reads fail when the catalog Workspace reader is missing; they never fall back to an unscoped listing.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- A Service with no catalog Workspace reader answers Browse with an error.
