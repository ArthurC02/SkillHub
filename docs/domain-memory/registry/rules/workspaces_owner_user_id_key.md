---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0002_identity.sql#L20-20"
    excerpt_sha256: "sha256:64a904c77b524f2061825afe85bdedac6da394b555bb2471388887cac301d4e2"
---
workspaces keeps unique-index workspaces_owner_user_id_key: CREATE UNIQUE INDEX workspaces_owner_user_id_key ON workspaces (owner_user_id)

# Relations
- Contexts: [identity](/contexts/identity)
