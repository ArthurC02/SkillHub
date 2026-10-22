---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0118_evaluation_batches.sql#L29-29"
    excerpt_sha256: "sha256:7d72b9de31f42cacaa161e07fd51a033a0f9e485ef2cae947190d5fd1de71511"
---
evaluation_batch_items keeps check evaluation_batch_items.position: position integer NOT NULL CHECK (position >= 0)

# Relations
- Contexts: [batch](/contexts/batch)
