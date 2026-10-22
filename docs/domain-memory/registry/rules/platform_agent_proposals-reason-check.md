---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L7-7"
    excerpt_sha256: "sha256:c75459873876099e1c1e26c167cf7a59b95828b7d803254687c1e9cadb47c48b"
---
platform_agent_proposals keeps check platform_agent_proposals.reason: reason text NOT NULL CHECK (btrim(reason) <> '')

# Relations
- Contexts: [operations](/contexts/operations)
