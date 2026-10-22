---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L26-26"
    excerpt_sha256: "sha256:f4ab4acd8c92ce1eed41c7ea11d9241e81b2c5781373dc15618f43bd37309dd6"
---
platform_agent_proposals keeps check platform_agent_proposals:26: CHECK (status <> 'failed' OR btrim(outcome) <> '')

# Relations
- Contexts: [operations](/contexts/operations)
