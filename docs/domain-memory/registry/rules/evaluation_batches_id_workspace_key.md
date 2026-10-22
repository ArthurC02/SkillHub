---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0118_evaluation_batches.sql#L12-12"
    excerpt_sha256: "sha256:3a20b13b4726a581bf93bfd8d159b8fc170512609728c2257953abf576dd0748"
---
evaluation_batches keeps unique evaluation_batches_id_workspace_key: CONSTRAINT evaluation_batches_id_workspace_key UNIQUE (id, workspace_id)

# Relations
- Contexts: [batch](/contexts/batch)
