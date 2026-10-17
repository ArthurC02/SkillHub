---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L166-172"
    excerpt_sha256: "sha256:9424e437ae4245042e062a777015cfc0533bedb669a587503a5dfaf10595fdb0"
  - resource: "apps/platform/internal/product/activity/service.go#L307-319"
    excerpt_sha256: "sha256:494b7302c857f8d127ba8b11ec41ac0022a9613d347917ce517d8916e5a9f20c"
---
A cursor the feed did not issue is refused as invalid before any owner source is read.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- cursor=not-a-cursor is refused and no reader is called.
