---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:41:43.641716Z"
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L178-181"
    excerpt_sha256: "sha256:ebf05fcb1b18546e76d0b540e5dc766bd3aaf2e988b49759d2cbc1b83b27c357"
  - resource: "apps/platform/internal/product/activity/service.go#L322-345"
    excerpt_sha256: "sha256:d24a699e771170a7089b09735765bb0a74fffbc07d074a1f4978e7ddd1471e7a"
  - resource: "apps/platform/internal/product/activity/service_test.go#L43-72"
    excerpt_sha256: "sha256:c928d01f5d0b0af2a2d6b54f1d8f038164c37567c527ed5fc400b1105ef9adc5"
---
The feed is ordered by priority, then newest first, then kind, then source id, and a cursor resumes strictly after the last item returned, so pages neither repeat nor skip an item.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- Items c (needs attention), a and b (recent, same time) with limit 2 page as [c, a] then [b].
