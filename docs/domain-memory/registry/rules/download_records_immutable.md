---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0027_packaging.sql#L56-58"
    excerpt_sha256: "sha256:b5c42bfc7587fcfe817195d1f768184d39327c4bc4113552efe26496ee5d7bef"
---
download_records keeps trigger download_records_immutable: CREATE TRIGGER download_records_immutable BEFORE UPDATE OR DELETE ON download_records FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [packaging](/contexts/packaging)
