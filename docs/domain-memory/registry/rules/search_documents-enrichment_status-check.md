---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0011_search_enrichment.sql#L1-8"
    excerpt_sha256: "sha256:4b9644ef621078f756d8a45439154674518f30eefa8843bb64660836214d320f"
---
search_documents keeps check search_documents.enrichment_status: enrichment_status text NOT NULL DEFAULT 'pending' CHECK (enrichment_status IN ('pending', 'enriched'))

# Relations
- Contexts: [catalog](/contexts/catalog)
