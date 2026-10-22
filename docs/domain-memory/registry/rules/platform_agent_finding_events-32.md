---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0112_platform_agent_findings.sql#L32-32"
    excerpt_sha256: "sha256:8f473358ef8e728b138960dc77c8a828b3bc06e41e9d475ff325f34e17c772f4"
---
platform_agent_finding_events keeps check platform_agent_finding_events:32: CHECK (operator_id IS NULL OR btrim(note) <> '')

# Relations
- Contexts: [operations](/contexts/operations)
