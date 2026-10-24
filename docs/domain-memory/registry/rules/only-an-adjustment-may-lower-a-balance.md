---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L370-371"
    excerpt_sha256: "sha256:037049a24ecd2317ebaa862dab5fb1f4c830d5d1685da0562920ef6340c87598"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L380-398"
    excerpt_sha256: "sha256:582d2b4adaa7a920bd3c99d1b16f71aa2efec1425e1c2f3d67e13a3c83ff2c61"
---
An operator entry with negative credits is refused with ErrGrantLowersBalance unless it is an adjustment.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- A grant of -10 is refused; the same -10 as an adjustment is applied.
