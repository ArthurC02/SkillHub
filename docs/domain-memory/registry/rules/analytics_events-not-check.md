---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0029_beta_analytics.sql#L14-15"
    excerpt_sha256: "sha256:674c559ec61bc2560c7220835ff612f8fb735278a656274964784753df98bb02"
---
analytics_events keeps check analytics_events.not: not a locale result_count integer CHECK (result_count IS NULL OR result_count >= 0)

# Relations
- Contexts: [analytics](/contexts/analytics)
