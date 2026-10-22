---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0049_account_purge_write_fence.sql#L44-46"
    excerpt_sha256: "sha256:f7822d58899bae51d567966df8090bb103d6147dab3f8bc8984ad2b0026822e4"
---
skills keeps trigger account_purge_update_fence: CREATE TRIGGER account_purge_update_fence BEFORE UPDATE ON skills FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge()

# Relations
- Contexts: [registry](/contexts/registry)
