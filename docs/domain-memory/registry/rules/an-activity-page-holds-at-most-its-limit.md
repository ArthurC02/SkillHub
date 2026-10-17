---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:41:43.641716Z"
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L182-187"
    excerpt_sha256: "sha256:cfae50c3f59c886c7d24aceeb0b4da7531e246d8cd936e9b2ec1368aa31a5e45"
  - resource: "apps/platform/internal/product/activity/service_test.go#L43-72"
    excerpt_sha256: "sha256:c928d01f5d0b0af2a2d6b54f1d8f038164c37567c527ed5fc400b1105ef9adc5"
---
A page never holds more items than its limit; a next cursor is issued only while items remain.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- Three items with limit 2: two items and a cursor, then one item and none.
