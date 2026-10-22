---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0033_evaluation_model_usage.sql#L1-2"
    excerpt_sha256: "sha256:b9f8ea4370b5584ae2dae732493391ac98ad40568ad0f01b8380220bd1ce6f1c"
---
evaluations keeps unique evaluations_id_workspace_key: CONSTRAINT evaluations_id_workspace_key UNIQUE (id, workspace_id)

# Relations
- Contexts: [eval](/contexts/eval)
