---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L76-76"
    excerpt_sha256: "sha256:8d0ccb07462c5a77c26207d4cbc7a267a1d4c8f4f4a07a72a4be5439c657deeb"
---
cost_statistics keeps check cost_statistics.window_end: window_end timestamptz NOT NULL CHECK (window_end > window_start)

# Relations
- Contexts: [credit](/contexts/credit)
