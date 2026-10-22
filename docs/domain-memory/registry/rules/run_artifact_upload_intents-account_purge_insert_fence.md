---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0051_unmanifested_object_cleanup_intents.sql#L44-46"
    excerpt_sha256: "sha256:cf3d5f10ae63024905d52431588b75014c61268ca297faffd201531ca7dcc24e"
---
run_artifact_upload_intents keeps trigger account_purge_insert_fence: CREATE TRIGGER account_purge_insert_fence BEFORE INSERT ON run_artifact_upload_intents FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge()

# Relations
- Contexts: [run](/contexts/run)
