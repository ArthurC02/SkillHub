---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0056_creation_sessions.sql#L46-47"
    excerpt_sha256: "sha256:d52eb58bad42ca5ea7d0ad25d76fce7b242f070960347304ee3aef74fae680fe"
---
creation_sessions keeps trigger account_purge_insert_fence: CREATE TRIGGER account_purge_insert_fence BEFORE INSERT ON creation_sessions FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge()

# Relations
- Contexts: [creation](/contexts/creation)
