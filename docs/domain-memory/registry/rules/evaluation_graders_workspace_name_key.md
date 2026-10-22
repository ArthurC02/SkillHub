---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0119_evaluation_graders.sql#L10-10"
    excerpt_sha256: "sha256:72cd7500eba1f43a7af68f042c8d9e7be06290c9e6f645327d55c8a318316f6a"
---
evaluation_graders keeps unique-index evaluation_graders_workspace_name_key: CREATE UNIQUE INDEX evaluation_graders_workspace_name_key ON evaluation_graders (workspace_id, name) WHERE workspace_id IS NOT NULL

# Relations
- Contexts: [eval](/contexts/eval)
