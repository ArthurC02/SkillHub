---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0029_beta_analytics.sql#L20-20"
    excerpt_sha256: "sha256:ab5a0cc6771a155be5d6c49b8e7eb31963066ee723835b3900bbf455ae10de07"
---
analytics_events keeps check analytics_events.arrival_rank: arrival_rank integer CHECK (arrival_rank IS NULL OR arrival_rank >= 1)

# Relations
- Contexts: [analytics](/contexts/analytics)
