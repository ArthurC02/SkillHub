---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0056_creation_sessions.sql#L50-51"
    excerpt_sha256: "sha256:40deca90fd413254939ebd6791efa1a75b5cce6b97dc532e9e60644823819673"
---
creation_session_events keeps trigger account_purge_insert_fence: CREATE TRIGGER account_purge_insert_fence BEFORE INSERT ON creation_session_events FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge()

# Relations
- Contexts: [creation](/contexts/creation)
