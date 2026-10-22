---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L29-30"
    excerpt_sha256: "sha256:1f6b3b693d015d810fe32ffbd5b89eba069019bf3dc5230e08aecc1f97166c7b"
---
platform_agent_proposals keeps unique-index platform_agent_proposals_one_live_idx: CREATE UNIQUE INDEX platform_agent_proposals_one_live_idx ON platform_agent_proposals (action) WHERE status IN ('proposed', 'approved', 'running')

# Relations
- Contexts: [operations](/contexts/operations)
