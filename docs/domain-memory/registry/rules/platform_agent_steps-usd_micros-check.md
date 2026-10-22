---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0111_platform_agent_steps.sql#L12-12"
    excerpt_sha256: "sha256:5496d65544a57ef62aae971db11e4f4b267d80d7be730fb4b77bd0e26d7f19b9"
---
platform_agent_steps keeps check platform_agent_steps.usd_micros: usd_micros bigint CHECK (usd_micros >= 0)

# Relations
- Contexts: [operations](/contexts/operations)
