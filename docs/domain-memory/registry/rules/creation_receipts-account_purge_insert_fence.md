---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0056_creation_sessions.sql#L52-53"
    excerpt_sha256: "sha256:2856e3e5def8bc625a67b882928cae07867484ae6e12d82633f93c68ae4485f5"
---
creation_receipts keeps trigger account_purge_insert_fence: CREATE TRIGGER account_purge_insert_fence BEFORE INSERT ON creation_receipts FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge()

# Relations
- Contexts: [creation](/contexts/creation)
