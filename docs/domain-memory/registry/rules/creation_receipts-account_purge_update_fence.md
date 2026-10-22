---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0056_creation_sessions.sql#L54-55"
    excerpt_sha256: "sha256:b250ff19ad3169bee582ea5866ed405fae9548e2653185a3bd4bbf08f12b8b6f"
---
creation_receipts keeps trigger account_purge_update_fence: CREATE TRIGGER account_purge_update_fence BEFORE UPDATE ON creation_receipts FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge()

# Relations
- Contexts: [creation](/contexts/creation)
