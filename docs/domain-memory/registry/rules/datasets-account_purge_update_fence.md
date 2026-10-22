---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0049_account_purge_write_fence.sql#L52-54"
    excerpt_sha256: "sha256:d14e55ef96beb188219bcf0c78d19a495ac254fed9b9c37610e43a9c80c605fe"
---
datasets keeps trigger account_purge_update_fence: CREATE TRIGGER account_purge_update_fence BEFORE UPDATE ON datasets FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge()

# Relations
- Contexts: [testlab](/contexts/testlab)
