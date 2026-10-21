---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:41:43.641716Z"
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L218-225"
    excerpt_sha256: "sha256:f2d1e366fa3772200e6b1bd8eb1e7d0bfcbb965686fec66250058ecbe4ea12ca"
  - resource: "apps/platform/internal/product/activity/service_test.go#L121-180"
    excerpt_sha256: "sha256:44c5252c0dd17e8f944521f43f7950a937b764cabedc8a4b044d8ab1753294bf"
---
Every owner source of the feed is read with the requesting Workspace's id, so no Workspace sees another's activity.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- Alice's Workspace lists her four items; Bob's lists none.
