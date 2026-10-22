---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L23-23"
    excerpt_sha256: "sha256:288d07e7dbafdcbd99a7e0136b1ab7dae11c7f4f522b1f97ba37c0345eec81f4"
---
platform_agent_proposals keeps check platform_agent_proposals:23: CHECK (decided_by IS NULL OR btrim(decision_note) <> '')

# Relations
- Contexts: [operations](/contexts/operations)
