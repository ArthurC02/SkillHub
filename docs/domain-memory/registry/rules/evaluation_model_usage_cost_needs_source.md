---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0033_evaluation_model_usage.sql#L16-17"
    excerpt_sha256: "sha256:231dd9db151bfc6038acec7012b9d74d72e522502eebdf61a025df46083a8793"
---
evaluation_model_usage keeps check evaluation_model_usage_cost_needs_source: CONSTRAINT evaluation_model_usage_cost_needs_source CHECK ((cost_usd IS NULL) = (cost_source IS NULL))

# Relations
- Contexts: [eval](/contexts/eval)
