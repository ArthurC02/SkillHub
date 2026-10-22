---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0118_evaluation_batches.sql#L32-32"
    excerpt_sha256: "sha256:c9a52023ea357bc335c09ee37c5770de7ff4253b6d7a934cf6b8c0c8496212e5"
---
evaluation_batch_items keeps unique evaluation_batch_items.run_id: run_id uuid UNIQUE

# Relations
- Contexts: [batch](/contexts/batch)
