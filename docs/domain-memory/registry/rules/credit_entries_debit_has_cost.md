---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L53-58"
    excerpt_sha256: "sha256:3ee1ef942adc20133fffe63463a7779dc7fc44fe11b35fb70f1035888a111bbd"
---
credit_entries keeps check credit_entries_debit_has_cost: CONSTRAINT credit_entries_debit_has_cost CHECK ( kind <> 'debit' OR ( delta_credits < 0 AND usd_micros IS NOT NULL AND markup_bps IS NOT NULL AND cost_event_id IS NOT NULL ) )

# Relations
- Contexts: [credit](/contexts/credit)
