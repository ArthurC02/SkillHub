---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L40-40"
    excerpt_sha256: "sha256:e6e3fc9271d9883e5173f3d7dcb9adc92356da079d5a3a047f518cfda3449cb4"
---
credit_entries keeps check credit_entries.delta_credits: delta_credits bigint NOT NULL CHECK (delta_credits <> 0)

# Relations
- Contexts: [credit](/contexts/credit)
