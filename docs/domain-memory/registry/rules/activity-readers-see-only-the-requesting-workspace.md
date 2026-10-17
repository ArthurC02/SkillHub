---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L218-225"
    excerpt_sha256: "sha256:f2d1e366fa3772200e6b1bd8eb1e7d0bfcbb965686fec66250058ecbe4ea12ca"
---
Every owner source of the feed is read with the requesting Workspace's id, so no Workspace sees another's activity.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- Alice's Workspace lists her four items; Bob's lists none.
