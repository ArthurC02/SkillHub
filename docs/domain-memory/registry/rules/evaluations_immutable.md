---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0086_failed_evaluations_immutable.sql#L3-7"
    excerpt_sha256: "sha256:fab30895513286d73a2f568c7c0bb64cceaff45e98aac7466174663d33a1ca73"
---
evaluations keeps trigger evaluations_immutable: CREATE TRIGGER evaluations_immutable BEFORE UPDATE OR DELETE ON evaluations FOR EACH ROW WHEN (OLD.status IN ('completed', 'failed')) EXECUTE FUNCTION enforce_immutable( 'feedback_helpful', 'feedback_comment', 'superseded_at', 'updated_at')

# Relations
- Contexts: [eval](/contexts/eval)
