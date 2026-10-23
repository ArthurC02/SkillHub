---
type: Interaction
status: draft
consistency: synchronous
delivery: "workspace-scoped foreign key; a batch is deleted with the skill version it pins"
sources:
  - resource: "db/migrations/0118_evaluation_batches.sql#L13-16"
    excerpt_sha256: "sha256:0653a3cb8aa469832ecf0022862f6b14fa7a26fbf3654a6fe0207cf5e1f15b1f"
---
# Relations
- Producer context: [registry](/contexts/registry)
- Consumer context: [batch](/contexts/batch)
