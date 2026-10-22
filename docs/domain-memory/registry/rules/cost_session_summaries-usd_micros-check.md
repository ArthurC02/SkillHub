---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0063_credit_session_summaries.sql#L5-5"
    excerpt_sha256: "sha256:511e0ee18436c6028238783ea196dd37d36cc461c0a2607fb69cc19ac172e3e2"
---
cost_session_summaries keeps check cost_session_summaries.usd_micros: usd_micros bigint NOT NULL CHECK (usd_micros >= 0)

# Relations
- Contexts: [credit](/contexts/credit)
