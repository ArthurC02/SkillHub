---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L14-15"
    excerpt_sha256: "sha256:602c9b7d84ffe9f17d23b470db2faad75b6931500ff92ee3ba4d643ba71170fa"
---
cost_events keeps check cost_events.ref_type: ref_type text CHECK (ref_type IS NULL OR ref_type IN ( 'creation_session', 'run', 'skill_version'))

# Relations
- Contexts: [credit](/contexts/credit)
