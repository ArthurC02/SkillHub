---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0033_evaluation_model_usage.sql#L28-30"
    excerpt_sha256: "sha256:e6494457051167ed38a8b9a9bbb9cdbc1b8641ac7e8dbc8c142ccf95981cec3e"
---
evaluation_model_usage keeps trigger evaluation_model_usage_immutable: CREATE TRIGGER evaluation_model_usage_immutable BEFORE UPDATE OR DELETE ON evaluation_model_usage FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [eval](/contexts/eval)
