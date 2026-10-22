---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0119_evaluation_graders.sql#L26-28"
    excerpt_sha256: "sha256:9b5404d54f2a2ac28b8193acaf9c91029af9f6f31b681571a8e1e9ce4a55a6f6"
---
evaluation_grader_versions keeps trigger evaluation_grader_versions_immutable: CREATE TRIGGER evaluation_grader_versions_immutable BEFORE UPDATE OR DELETE ON evaluation_grader_versions FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [eval](/contexts/eval)
