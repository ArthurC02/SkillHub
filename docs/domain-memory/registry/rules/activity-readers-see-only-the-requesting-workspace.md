---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:41:43.641716Z"
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L218-225"
    excerpt_sha256: "sha256:f2d1e366fa3772200e6b1bd8eb1e7d0bfcbb965686fec66250058ecbe4ea12ca"
  - resource: "apps/platform/internal/product/activity/service_test.go#L121-179"
    excerpt_sha256: "sha256:f221aa575c58692e8d7efee541d3f73625e24c4e6f3f0736d6019413cdd53c5e"
---
Every owner source of the feed is read with the requesting Workspace's id, so no Workspace sees another's activity.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- Alice's Workspace lists her four items; Bob's lists none.
