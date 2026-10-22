---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L83-83"
    excerpt_sha256: "sha256:e7433d7c71429e0a1a7636f3cd0cb23f074b9d30242b4a35adb1c5a7f6635f51"
---
cost_statistics keeps unique cost_statistics_kind_window_end_key: CONSTRAINT cost_statistics_kind_window_end_key UNIQUE (kind, window_end)

# Relations
- Contexts: [credit](/contexts/credit)
