---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0071_suggestion_version_provenance.sql#L32-34"
    excerpt_sha256: "sha256:588d6024b1669315fefdadf11da7476ab39f715ca0f093f46151909978b9a2e0"
---
evaluation_suggestion_applications keeps trigger account_purge_insert_fence: CREATE TRIGGER account_purge_insert_fence BEFORE INSERT ON evaluation_suggestion_applications FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge()

# Relations
- Contexts: [eval](/contexts/eval)
