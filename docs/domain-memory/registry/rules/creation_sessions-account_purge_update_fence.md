---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0056_creation_sessions.sql#L48-49"
    excerpt_sha256: "sha256:0459fd082eb6ffc9a8a703fb1178e2386c42c23d8c71d14976f3720ac1d88848"
---
creation_sessions keeps trigger account_purge_update_fence: CREATE TRIGGER account_purge_update_fence BEFORE UPDATE ON creation_sessions FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge()

# Relations
- Contexts: [creation](/contexts/creation)
