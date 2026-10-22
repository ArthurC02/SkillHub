---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0109_platform_agents.sql#L6-6"
    excerpt_sha256: "sha256:6e69ec461341ba7b7ced5bc505f2fca7ef79b42b24ab04f3a906a40e6e9ce9cd"
---
platform_agents keeps check platform_agents.daily_spend_cap_micros: daily_spend_cap_micros bigint NOT NULL CHECK (daily_spend_cap_micros > 0)

# Relations
- Contexts: [operations](/contexts/operations)
