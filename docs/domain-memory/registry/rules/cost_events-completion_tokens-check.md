---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L9-9"
    excerpt_sha256: "sha256:3e47502e2bd09680cdc9d9f8dcfb6aea318ec8718657612edec81d196f2aa973"
---
cost_events keeps check cost_events.completion_tokens: completion_tokens bigint NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0)

# Relations
- Contexts: [credit](/contexts/credit)
