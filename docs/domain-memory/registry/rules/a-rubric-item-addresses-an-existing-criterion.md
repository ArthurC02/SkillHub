---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:46:14.276516Z"
sources:
  - resource: "apps/platform/internal/trial/design/testlab.go#L238-251"
    excerpt_sha256: "sha256:a9090bc6e32c38085d4e72a23ca3a904979ad2d35f984125d4ec197f4ff466aa"
  - resource: "apps/platform/internal/trial/design/testlab.go#L733-761"
    excerpt_sha256: "sha256:4ef606451bd38712bf45d5464617d19abc8ad86571407c22153130bbe9beff5e"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/testlab_integration_test.go#L328-389"
    excerpt_sha256: "sha256:5dd4d78fbfbedd376541466baa1926b09eaf3753e4d1251d5cfb58b26d243cb0"
---
A rubric is accepted only when every item names an existing acceptance criterion, and deleting a criterion removes its rubric item.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A rubric item naming not-a-criterion answers 400.
