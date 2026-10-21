---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:41:43.641716Z"
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L189-193"
    excerpt_sha256: "sha256:d2f980e43612036fe79fa7371e36fd3848852f6d61df693c61049edea288774f"
  - resource: "apps/platform/internal/product/activity/service_test.go#L43-72"
    excerpt_sha256: "sha256:7353aed81e25babc2467fdb7956d7429dedcb4ffb80b05f1c0c0337e37d2961c"
---
A page never holds more items than its limit; a next cursor is issued only while items remain.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- Three items with limit 2: two items and a cursor, then one item and none.
