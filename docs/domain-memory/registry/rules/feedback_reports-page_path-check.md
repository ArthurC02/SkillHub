---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0029_beta_analytics.sql#L41-41"
    excerpt_sha256: "sha256:84e5e02454930f1644e125427d8d970da0bad013b32b4e3414b3e1f9f40ebe63"
---
feedback_reports keeps check feedback_reports.page_path: page_path text CHECK (page_path IS NULL OR length(page_path) <= 512)

# Relations
- Contexts: [analytics](/contexts/analytics)
