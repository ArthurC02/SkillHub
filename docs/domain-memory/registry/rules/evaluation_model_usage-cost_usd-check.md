---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0033_evaluation_model_usage.sql#L13-13"
    excerpt_sha256: "sha256:0b6eed90a663f8c5d085ac03cfa4272945cef2c459443bf64fe921f45a48e238"
---
evaluation_model_usage keeps check evaluation_model_usage.cost_usd: cost_usd numeric(12, 6) CHECK (cost_usd IS NULL OR cost_usd >= 0)

# Relations
- Contexts: [eval](/contexts/eval)
