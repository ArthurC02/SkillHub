---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0116_operator_platform_settings.sql#L1-2"
    excerpt_sha256: "sha256:6f11775fd4dc477745f10c53a687a80134af0a600839e1607c901783c11695b3"
---
platform_agents keeps check platform_agents.daily_spend_cap_override_micros: daily_spend_cap_override_micros bigint CHECK (daily_spend_cap_override_micros > 0)

# Relations
- Contexts: [operations](/contexts/operations)
