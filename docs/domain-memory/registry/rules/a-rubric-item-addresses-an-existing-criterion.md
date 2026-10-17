---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/testlab.go#L238-251"
    excerpt_sha256: "sha256:a9090bc6e32c38085d4e72a23ca3a904979ad2d35f984125d4ec197f4ff466aa"
  - resource: "apps/platform/internal/trial/design/testlab.go#L733-761"
    excerpt_sha256: "sha256:4ef606451bd38712bf45d5464617d19abc8ad86571407c22153130bbe9beff5e"
---
A rubric is accepted only when every item names an existing acceptance criterion, and deleting a criterion removes its rubric item.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A rubric item naming not-a-criterion answers 400.
