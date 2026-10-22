---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L8-8"
    excerpt_sha256: "sha256:31d664f4dbf776cd086a719ea3dd68ea3a60d9ba2e49eacddc89c5a0f49f4749"
---
platform_agent_proposals keeps check platform_agent_proposals.cites: cites text[] NOT NULL CHECK (cardinality(cites) > 0)

# Relations
- Contexts: [operations](/contexts/operations)
