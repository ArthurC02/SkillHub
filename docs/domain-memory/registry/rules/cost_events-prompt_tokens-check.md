---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L8-8"
    excerpt_sha256: "sha256:16b0a5d7ae0a6d8223e39ae10a636ebb1cda8262a08d434b1b717b6268f3a2f5"
---
cost_events keeps check cost_events.prompt_tokens: prompt_tokens bigint NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0)

# Relations
- Contexts: [credit](/contexts/credit)
