DROP TRIGGER evaluations_immutable ON evaluations;

CREATE TRIGGER evaluations_immutable
    BEFORE UPDATE OR DELETE ON evaluations
    FOR EACH ROW WHEN (OLD.status IN ('completed', 'failed'))
    EXECUTE FUNCTION enforce_immutable(
        'feedback_helpful', 'feedback_comment', 'superseded_at', 'updated_at');
