---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0029_beta_analytics.sql#L3-8"
    excerpt_sha256: "sha256:d2483f2d53286ebdbc5586f551a3044aab6c5e02beff24018184d6e83b6b003d"
---
analytics_events keeps check analytics_events.event_name: event_name text NOT NULL CHECK (event_name IN ( 'search_performed', 'skill_detail_viewed', 'session_started', 'download_started' ))

# Relations
- Contexts: [analytics](/contexts/analytics)
