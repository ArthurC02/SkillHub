---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L182-187"
    excerpt_sha256: "sha256:cfae50c3f59c886c7d24aceeb0b4da7531e246d8cd936e9b2ec1368aa31a5e45"
---
A page never holds more items than its limit; a next cursor is issued only while items remain.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- Three items with limit 2: two items and a cursor, then one item and none.
