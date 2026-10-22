---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L42-42"
    excerpt_sha256: "sha256:4d6230621a7b89051308dac0a8167ca27113943eab91782956d93c5bb22dfb9f"
---
credit_entries keeps check credit_entries.markup_bps: markup_bps integer CHECK (markup_bps IS NULL OR markup_bps >= 0)

# Relations
- Contexts: [credit](/contexts/credit)
