---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0033_evaluation_model_usage.sql#L12-12"
    excerpt_sha256: "sha256:2b0197cdee7199c4f01bba6ce44f6679ab69210731964644f60936ce7a20b751"
---
evaluation_model_usage keeps check evaluation_model_usage.completion_tokens: completion_tokens bigint NOT NULL CHECK (completion_tokens >= 0)

# Relations
- Contexts: [eval](/contexts/eval)
