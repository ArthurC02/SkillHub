---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0071_suggestion_version_provenance.sql#L28-30"
    excerpt_sha256: "sha256:12ef03ed4680f2177058068cdaa88ff434fb9d95578777052bbec4664a62b2ca"
---
evaluation_suggestion_applications keeps trigger evaluation_suggestion_applications_immutable: CREATE TRIGGER evaluation_suggestion_applications_immutable BEFORE UPDATE OR DELETE ON evaluation_suggestion_applications FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [eval](/contexts/eval)
