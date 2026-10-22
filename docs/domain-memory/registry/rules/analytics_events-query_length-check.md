---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0029_beta_analytics.sql#L13-13"
    excerpt_sha256: "sha256:349a534301450c82150d4b3134413b9ef226ef93d0a82c15aa686286b312b0d9"
---
analytics_events keeps check analytics_events.query_length: query_length integer CHECK (query_length IS NULL OR query_length >= 0)

# Relations
- Contexts: [analytics](/contexts/analytics)
