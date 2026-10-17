---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/discovery/enrichment.go#L12-14"
    excerpt_sha256: "sha256:c8086abe2eb931f39133735544a49735766e725679ad1a32ddc72e9620f779b2"
  - resource: "apps/platform/internal/skill/discovery/projection.go#L109-109"
    excerpt_sha256: "sha256:199c4d254986e18f47af0f6f37dbfd906346d604dd293e8696da7c047ac560a2"
  - resource: "apps/platform/internal/skill/discovery/projection_test.go#L26-44"
    excerpt_sha256: "sha256:da55aa942302f94559e17233d087426deafb687be03efd9c32840365fb2c56c4"
---
A search document is listable only once its enrichment is finished or it has an embedding a vector search can find.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- A pending document without a vector is not listable; the same document with a vector is.
