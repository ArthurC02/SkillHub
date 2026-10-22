---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0027_packaging.sql#L35-37"
    excerpt_sha256: "sha256:5225aff44eea8d5d27f17a4fc426094f28466c261f2eec7b95f589834699823a"
---
download_artifacts keeps trigger download_artifacts_immutable: CREATE TRIGGER download_artifacts_immutable BEFORE UPDATE OR DELETE ON download_artifacts FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [packaging](/contexts/packaging)
