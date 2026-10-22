---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0029_beta_analytics.sql#L40-40"
    excerpt_sha256: "sha256:2be00c921363f25896106ea9e310ea79ef7aad4638393a19592b1ed0483cb1d0"
---
feedback_reports keeps check feedback_reports.message: message text NOT NULL CHECK (length(message) BETWEEN 1 AND 2000)

# Relations
- Contexts: [analytics](/contexts/analytics)
