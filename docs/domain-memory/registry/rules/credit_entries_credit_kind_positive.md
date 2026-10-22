---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L59-61"
    excerpt_sha256: "sha256:7c3b7d6d14e147755ec855234518918c18632d1893aa50b98d84a6f58dd6b408"
---
credit_entries keeps check credit_entries_credit_kind_positive: CONSTRAINT credit_entries_credit_kind_positive CHECK ( kind NOT IN ('grant', 'topup') OR delta_credits > 0 )

# Relations
- Contexts: [credit](/contexts/credit)
