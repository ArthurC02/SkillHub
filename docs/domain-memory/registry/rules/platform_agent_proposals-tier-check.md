---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L6-6"
    excerpt_sha256: "sha256:b59165fc6673b493defaaeeb4741d87d19a86773ce4f5c2de6b92982ff3e9071"
---
platform_agent_proposals keeps check platform_agent_proposals.tier: tier text NOT NULL CHECK (tier IN ('read_only', 'reversible', 'destructive'))

# Relations
- Contexts: [operations](/contexts/operations)
