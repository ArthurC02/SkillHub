---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0033_evaluation_model_usage.sql#L18-19"
    excerpt_sha256: "sha256:bcad26fbca39f843363f921e2bb79708cfee3863638a61cd930dc8693c13454a"
---
evaluation_model_usage keeps unique evaluation_model_usage_once_per_operation: CONSTRAINT evaluation_model_usage_once_per_operation UNIQUE (evaluation_id, operation)

# Relations
- Contexts: [eval](/contexts/eval)
