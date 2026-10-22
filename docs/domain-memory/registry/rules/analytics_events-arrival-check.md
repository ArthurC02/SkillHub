---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0029_beta_analytics.sql#L19-19"
    excerpt_sha256: "sha256:066244aff368407531842615cf0cd1683fb6f82ba789de3f2159ff1606d60367"
---
analytics_events keeps check analytics_events.arrival: arrival text CHECK (arrival IS NULL OR arrival IN ('search', 'direct'))

# Relations
- Contexts: [analytics](/contexts/analytics)
