---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L52-52"
    excerpt_sha256: "sha256:df539fe69dffd7af1f90d75aa509c0a3a68ae83c358dfedd6db3a84ab5d8a9c3"
---
credit_entries keeps unique credit_entries_idempotency_key_key: CONSTRAINT credit_entries_idempotency_key_key UNIQUE (idempotency_key)

# Relations
- Contexts: [credit](/contexts/credit)
