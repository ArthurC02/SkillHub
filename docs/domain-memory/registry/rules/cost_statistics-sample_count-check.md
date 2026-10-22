---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L77-77"
    excerpt_sha256: "sha256:c08c0d45a0b57ae68d0f34d07b9f0c0384d6adc863732965fe8dc08570bfc560"
---
cost_statistics keeps check cost_statistics.sample_count: sample_count bigint NOT NULL CHECK (sample_count >= 0)

# Relations
- Contexts: [credit](/contexts/credit)
