---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L32-32"
    excerpt_sha256: "sha256:3e1309e80578b381095ace48b07c56dbde54812855537956339e62a39796fb50"
---
credit_accounts keeps check credit_accounts.balance_credits: balance_credits bigint NOT NULL DEFAULT 0 CHECK (balance_credits >= -1000000)

# Relations
- Contexts: [credit](/contexts/credit)
