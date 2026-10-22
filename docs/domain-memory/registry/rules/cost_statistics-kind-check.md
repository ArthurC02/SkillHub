---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L72-74"
    excerpt_sha256: "sha256:4418b4b769d308581f2bb45415d4bd0cb9b7a8d90b2b60e14c78311179c22b4c"
---
cost_statistics keeps check cost_statistics.kind: kind text NOT NULL CHECK (kind IN ( 'creation_step', 'search_embedding', 'index_enrich', 'review', 'suggestion', 'generate'))

# Relations
- Contexts: [credit](/contexts/credit)
