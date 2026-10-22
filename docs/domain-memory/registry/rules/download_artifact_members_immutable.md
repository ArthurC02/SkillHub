---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0088_bundles.sql#L77-79"
    excerpt_sha256: "sha256:64227beaa5ea87b49948b5040d4f2b604eec404acb36e4a650b46ab7d82fdcb8"
---
download_artifact_members keeps trigger download_artifact_members_immutable: CREATE TRIGGER download_artifact_members_immutable BEFORE UPDATE OR DELETE ON download_artifact_members FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [packaging](/contexts/packaging)
