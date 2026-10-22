---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L39-39"
    excerpt_sha256: "sha256:ee2468e353d9d48c80af4eb07fe2b2d7e5935ae01204b5855a51402df5614d4a"
---
credit_entries keeps check credit_entries.kind: kind text NOT NULL CHECK (kind IN ('debit', 'grant', 'topup', 'adjustment'))

# Relations
- Contexts: [credit](/contexts/credit)
