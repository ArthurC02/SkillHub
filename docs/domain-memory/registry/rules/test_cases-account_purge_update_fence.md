---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0049_account_purge_write_fence.sql#L48-50"
    excerpt_sha256: "sha256:0d7957e6621d3ea139de559edf4e00d4ec526bdffb8b7f50f09751c382046dd2"
---
test_cases keeps trigger account_purge_update_fence: CREATE TRIGGER account_purge_update_fence BEFORE UPDATE ON test_cases FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge()

# Relations
- Contexts: [testlab](/contexts/testlab)
