---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0029_beta_analytics.sql#L39-39"
    excerpt_sha256: "sha256:f0e9393fe53979bd65f78640ee610aac43cc791c930e563751f8f82e25cc22fe"
---
feedback_reports keeps check feedback_reports.kind: kind text NOT NULL CHECK (kind IN ('blocking_issue', 'need_signal'))

# Relations
- Contexts: [analytics](/contexts/analytics)
