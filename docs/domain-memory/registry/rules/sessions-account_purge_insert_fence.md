---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0049_account_purge_write_fence.sql#L82-84"
    excerpt_sha256: "sha256:fc76d88f53326bf7a2fab7f40beefbd5802f39e5d30b5ad589b5d6534eb7ca1b"
---
sessions keeps trigger account_purge_insert_fence: CREATE TRIGGER account_purge_insert_fence BEFORE INSERT ON sessions FOR EACH ROW EXECUTE FUNCTION fence_session_write_during_account_purge()

# Relations
- Contexts: [identity](/contexts/identity)
