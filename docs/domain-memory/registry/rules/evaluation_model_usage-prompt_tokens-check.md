---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0033_evaluation_model_usage.sql#L11-11"
    excerpt_sha256: "sha256:8c9c0dd41f155381c8b2f37e0eee3c25e688b68258bca8419140c5d383c4680e"
---
evaluation_model_usage keeps check evaluation_model_usage.prompt_tokens: prompt_tokens bigint NOT NULL CHECK (prompt_tokens >= 0)

# Relations
- Contexts: [eval](/contexts/eval)
