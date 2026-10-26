---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-26T01:48:28.064585Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/enrichment.go#L12-14"
    excerpt_sha256: "sha256:c8086abe2eb931f39133735544a49735766e725679ad1a32ddc72e9620f779b2"
  - resource: "apps/platform/internal/skill/discovery/projection.go#L74-74"
    excerpt_sha256: "sha256:199c4d254986e18f47af0f6f37dbfd906346d604dd293e8696da7c047ac560a2"
  - resource: "apps/platform/internal/skill/discovery/projection_test.go#L26-44"
    excerpt_sha256: "sha256:da55aa942302f94559e17233d087426deafb687be03efd9c32840365fb2c56c4"
  - resource: "apps/platform/internal/skill/admission/enrich.go#L144-145"
    excerpt_sha256: "sha256:1102bebb3ecf7d70ed2069e46b3cd3a67d8909cf1f29363fda139f1efc05d171"
  - resource: "apps/platform/internal/skill/discovery/enrichment_backfill.go#L64-85"
    excerpt_sha256: "sha256:6f4467a736a9cac9d68b2d291db75297d5c741362b5257dc9bb88b457a2765d0"
  - resource: "db/queries/search.sql#L54-70"
    excerpt_sha256: "sha256:3de7d1355ee0226bf147d60b96fd8fa02d3b1723ec01683dfc563202ee113c30"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/search_embedding_backfill_integration_test.go#L47-84"
    excerpt_sha256: "sha256:8ee0514e3687b512bd5ad4fdc9b4d44fb5305de500808404ce53e7eef7123611"
---
A search document is listable only once its enrichment is finished or it has an embedding a vector search can find. An enrichment whose embedding call failed is still stored as enriched, without a vector, so it is listable at once; the enrichment backfill later embeds that stored text without calling the enrichment model again, and stores the vector only if the document has not been rewritten since it was claimed.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- A pending document without a vector is not listable; the same document with a vector is.
- The enrichment answers and the embedding call fails: the document is enriched and listable without a vector; the next backfill pass makes one embedding call and no enrichment call, and a vector computed before a re-import rewrote the document is dropped.
