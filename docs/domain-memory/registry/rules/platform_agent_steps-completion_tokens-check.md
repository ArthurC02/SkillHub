---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0111_platform_agent_steps.sql#L11-11"
    excerpt_sha256: "sha256:3e47502e2bd09680cdc9d9f8dcfb6aea318ec8718657612edec81d196f2aa973"
---
platform_agent_steps keeps check platform_agent_steps.completion_tokens: completion_tokens bigint NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0)

# Relations
- Contexts: [operations](/contexts/operations)
