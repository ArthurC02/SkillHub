---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L180-183"
    excerpt_sha256: "sha256:5016a6ac6b4593356e8f1043e78fbe7859ff1d979ba065c1d26bf32f557bd694"
  - resource: "apps/platform/internal/product/activity/service.go#L269-271"
    excerpt_sha256: "sha256:24e1a8bc3ae06fff8ebb02fa622c923f4a37aa0640b0a7701e1c0119b1481c28"
  - resource: "apps/platform/internal/product/activity/service_test.go#L74-93"
    excerpt_sha256: "sha256:b1ea57ac9aba394d7956bcf7b1be726bfa635dd5696bcf7ecf9502b3d6b62fb1"
---
If any owner source of the feed cannot be read or is not configured, no partial page is returned; the request fails naming every source that failed.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- The evaluation store errors and the publishing reader is missing: the error names both and no items are returned.
