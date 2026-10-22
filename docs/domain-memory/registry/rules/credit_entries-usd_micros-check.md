---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L41-41"
    excerpt_sha256: "sha256:fc7981101814c8b79d14475ae139fa39a1517a24fe3adc15b690da59b3071f2c"
---
credit_entries keeps check credit_entries.usd_micros: usd_micros bigint CHECK (usd_micros IS NULL OR usd_micros >= 0)

# Relations
- Contexts: [credit](/contexts/credit)
