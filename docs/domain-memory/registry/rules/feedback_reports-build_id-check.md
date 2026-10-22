---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0054_feedback_build_id.sql#L1-2"
    excerpt_sha256: "sha256:ff749c56ae002b070af4edadab7f9bb055222296e73f13a310894f0c598820db"
---
feedback_reports keeps check feedback_reports.build_id: build_id text CHECK (build_id IS NULL OR length(build_id) <= 64)

# Relations
- Contexts: [analytics](/contexts/analytics)
