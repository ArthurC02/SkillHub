---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L19-19"
    excerpt_sha256: "sha256:d9b57ddca443f01724f73f46112ac86c57cfa58569451d0b803d9df7a84c449b"
---
cost_events keeps unique cost_events_idempotency_key_key: CONSTRAINT cost_events_idempotency_key_key UNIQUE (idempotency_key)

# Relations
- Contexts: [credit](/contexts/credit)
