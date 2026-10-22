---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0126_feedback_erased_on_purge.sql#L3-5"
    excerpt_sha256: "sha256:d350201acceec43525d45c93562e06de916a39682d28ff47738343e70587c5ad"
---
feedback_reports keeps check feedback_reports_message_kept_while_attributed: CONSTRAINT feedback_reports_message_kept_while_attributed CHECK (message IS NOT NULL OR (workspace_id IS NULL AND user_id IS NULL))

# Relations
- Contexts: [analytics](/contexts/analytics)
