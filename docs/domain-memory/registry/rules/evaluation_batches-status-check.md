---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0118_evaluation_batches.sql#L7-7"
    excerpt_sha256: "sha256:f5e6976396c5228d75717faf713bfc003da44596942e9c3ac8323fc04f549de6"
---
evaluation_batches keeps check evaluation_batches.status: status text NOT NULL DEFAULT 'launching' CHECK (status IN ('launching', 'launched', 'stopped'))

# Relations
- Contexts: [batch](/contexts/batch)
