---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0111_platform_agent_steps.sql#L10-10"
    excerpt_sha256: "sha256:16b0a5d7ae0a6d8223e39ae10a636ebb1cda8262a08d434b1b717b6268f3a2f5"
---
platform_agent_steps keeps check platform_agent_steps.prompt_tokens: prompt_tokens bigint NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0)

# Relations
- Contexts: [operations](/contexts/operations)
