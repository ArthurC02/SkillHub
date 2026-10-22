---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L11-11"
    excerpt_sha256: "sha256:0ef3f0ee8b3647168fd80f95a4ca760dd15452a3fd4daf8d49c93dad5a5bd51f"
---
cost_events keeps check cost_events.cost_source: cost_source text NOT NULL CHECK (cost_source IN ('gateway', 'estimated'))

# Relations
- Contexts: [credit](/contexts/credit)
