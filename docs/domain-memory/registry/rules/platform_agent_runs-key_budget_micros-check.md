---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0115_platform_agent_run_key_budget.sql#L1-1"
    excerpt_sha256: "sha256:79996971540e13c102df051b949cfbaa3ce2f6654138cd934831f47795526a21"
---
platform_agent_runs keeps check platform_agent_runs.key_budget_micros: key_budget_micros bigint CHECK (key_budget_micros > 0)

# Relations
- Contexts: [operations](/contexts/operations)
