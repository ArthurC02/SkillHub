---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0121_simulated_user.sql#L8-9"
    excerpt_sha256: "sha256:08c92a580aa024b0a84c4ce9f0035219387f8839c0ba509a8073e57f3958915b"
---
evaluation_batches keeps check evaluation_batches_settled_once_launched: CONSTRAINT evaluation_batches_settled_once_launched CHECK (settled_at IS NULL OR status = 'launched')

# Relations
- Contexts: [batch](/contexts/batch)
