---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0051_unmanifested_object_cleanup_intents.sql#L19-21"
    excerpt_sha256: "sha256:9c68fc074ca9d833dadf8ed94abb59c47dd7a21abfa7a26aab3cbf742c282a14"
---
download_object_cleanup_intents keeps trigger account_purge_insert_fence: CREATE TRIGGER account_purge_insert_fence BEFORE INSERT ON download_object_cleanup_intents FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge()

# Relations
- Contexts: [packaging](/contexts/packaging)
