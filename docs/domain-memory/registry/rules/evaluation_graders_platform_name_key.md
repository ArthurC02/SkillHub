---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0119_evaluation_graders.sql#L9-9"
    excerpt_sha256: "sha256:b87f0db2196298e0ba429d798690ea8e398e98d6a2fdf9b929f58ee6489338dc"
---
evaluation_graders keeps unique-index evaluation_graders_platform_name_key: CREATE UNIQUE INDEX evaluation_graders_platform_name_key ON evaluation_graders (name) WHERE workspace_id IS NULL

# Relations
- Contexts: [eval](/contexts/eval)
