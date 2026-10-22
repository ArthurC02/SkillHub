---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0109_platform_agents.sql#L3-3"
    excerpt_sha256: "sha256:3eb74b82e92e117fa22982d60c03caed7f950715fda539f9802d004737c437e8"
---
platform_agents keeps unique platform_agents.name: name text NOT NULL UNIQUE CHECK (name ~ '^[a-z][a-z0-9-]*$')

# Relations
- Contexts: [operations](/contexts/operations)
