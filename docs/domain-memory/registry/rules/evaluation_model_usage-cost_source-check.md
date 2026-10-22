---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0033_evaluation_model_usage.sql#L14-14"
    excerpt_sha256: "sha256:6954c17cc0742552dca110f4b7ee4d4f4ad231ef03428639a7f06664a0f17b38"
---
evaluation_model_usage keeps check evaluation_model_usage.cost_source: cost_source text CHECK (cost_source IN ('gateway', 'estimated'))

# Relations
- Contexts: [eval](/contexts/eval)
