---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L189-189"
    excerpt_sha256: "sha256:69b248ad4ff7adbef9e2c0e03438a952e2ccfbe3a7f716d0335f1e71fa7eb319"
  - resource: "apps/platform/internal/product/activity/service_test.go#L111-119"
    excerpt_sha256: "sha256:2bb4dcee50a53aca777c13801fa09118b88f2d08084562322badf98f2dd9b3d4"
---
A page the activity service serves says it is complete and names all contracted sources, even when it holds no items.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- An empty workspace gets complete=true with the five sources listed.
