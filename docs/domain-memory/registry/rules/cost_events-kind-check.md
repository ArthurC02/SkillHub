---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L3-5"
    excerpt_sha256: "sha256:51f8ca629066f2655bb23959b8edb02ccfa6873154d8f1e32c65cbc966475c49"
---
cost_events keeps check cost_events.kind: kind text NOT NULL CHECK (kind IN ( 'creation_step', 'search_embedding', 'index_enrich', 'review', 'suggestion', 'generate'))

# Relations
- Contexts: [credit](/contexts/credit)
