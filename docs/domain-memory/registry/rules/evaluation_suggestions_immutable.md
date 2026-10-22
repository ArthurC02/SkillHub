---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0024_evaluation.sql#L87-90"
    excerpt_sha256: "sha256:99514dbd11cacebaa9413a321b8f045760858b612954b8ac6218c4c93ea1c064"
---
evaluation_suggestions keeps trigger evaluation_suggestions_immutable: CREATE TRIGGER evaluation_suggestions_immutable BEFORE UPDATE OR DELETE ON evaluation_suggestions FOR EACH ROW EXECUTE FUNCTION enforce_immutable( 'decision', 'decided_at', 'applied_skill_version_id')

# Relations
- Contexts: [eval](/contexts/eval)
