---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0118_evaluation_batches.sql#L30-30"
    excerpt_sha256: "sha256:89140483f67b4b61c9df05524fcdd18b1a3a553d0ce7f4ac7c3b41fee8ec0322"
---
evaluation_batch_items keeps check evaluation_batch_items.trial: trial integer NOT NULL CHECK (trial >= 1)

# Relations
- Contexts: [batch](/contexts/batch)
