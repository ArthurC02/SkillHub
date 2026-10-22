---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L10-10"
    excerpt_sha256: "sha256:ad1940420208144221e48c2d65cd000814a4c2b8a53d0583e0f670de7fea0198"
---
cost_events keeps check cost_events.usd_micros: usd_micros bigint NOT NULL CHECK (usd_micros >= 0)

# Relations
- Contexts: [credit](/contexts/credit)
