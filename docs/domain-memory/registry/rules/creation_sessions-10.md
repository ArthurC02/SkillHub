---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0056_creation_sessions.sql#L10-10"
    excerpt_sha256: "sha256:5dcd45afc996fb1c58bc792d37ba51ddb1f120065f6929c128b5e225d0fa674a"
---
creation_sessions keeps unique creation_sessions:10: UNIQUE(id, workspace_id)

# Relations
- Contexts: [creation](/contexts/creation)
