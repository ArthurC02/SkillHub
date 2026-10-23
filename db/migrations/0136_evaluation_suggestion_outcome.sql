ALTER TABLE evaluations ADD COLUMN suggestion_outcome text
    CONSTRAINT evaluations_suggestion_outcome_known CHECK (suggestion_outcome IN (
        'restricted', 'not_needed', 'no_evidence', 'unavailable', 'none_proposed', 'none_kept', 'kept'));

DROP TRIGGER evaluations_immutable ON evaluations;

CREATE TRIGGER evaluations_immutable
    BEFORE UPDATE OR DELETE ON evaluations
    FOR EACH ROW WHEN (OLD.status IN ('completed', 'failed'))
    EXECUTE FUNCTION enforce_immutable(
        'feedback_helpful', 'feedback_comment', 'superseded_at', 'updated_at', 'suggestion_outcome');

COMMENT ON COLUMN evaluations.suggestion_outcome IS
    'Why this evaluation has the improvement suggestions it has, written once after the verdict: restricted (the skill is under a licence hold), not_needed (the verdict leaves nothing to improve), no_evidence (the verdict cites nothing a suggestion could quote), unavailable (the suggestion model call failed), none_proposed (the model answered with no proposals), none_kept (every proposal failed evidence or storage checks), kept (at least one suggestion stored). Null when the deployment has no suggestion model or the evaluation predates this column.';
