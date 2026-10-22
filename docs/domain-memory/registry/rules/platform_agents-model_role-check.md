---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0109_platform_agents.sql#L5-5"
    excerpt_sha256: "sha256:bcf0aca74a4a06f5bf9a6a7e845c862566497dbc478690e1df9fe4c7f2ca242b"
---
platform_agents keeps check platform_agents.model_role: model_role text NOT NULL CHECK (btrim(model_role) <> '')

# Relations
- Contexts: [operations](/contexts/operations)
