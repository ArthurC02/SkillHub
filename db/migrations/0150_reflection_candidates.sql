ALTER TABLE evaluation_settings
    ADD COLUMN reflection_candidates smallint NOT NULL DEFAULT 1
        CONSTRAINT evaluation_settings_reflection_candidates_range CHECK (reflection_candidates BETWEEN 1 AND 4),
    ADD COLUMN reflection_round_credit_cap integer
        CONSTRAINT evaluation_settings_reflection_round_credit_cap_positive
        CHECK (reflection_round_credit_cap IS NULL OR reflection_round_credit_cap > 0),
    ADD COLUMN reflection_candidates_reason text
        CONSTRAINT evaluation_settings_reflection_candidates_reason_check
        CHECK (reflection_candidates_reason IS NULL OR btrim(reflection_candidates_reason) <> ''),
    ADD COLUMN reflection_candidates_set_at timestamptz,
    ADD CONSTRAINT evaluation_settings_reflection_candidates_change_is_explained
        CHECK ((reflection_candidates_reason IS NULL) = (reflection_candidates_set_at IS NULL));

COMMENT ON COLUMN evaluation_settings.reflection_candidates IS
    'How many alternative suggestion sets one batch improvement round asks for, each with its own focus. 1 is a single set with no focus.';
COMMENT ON COLUMN evaluation_settings.reflection_round_credit_cap IS
    'The most credits one batch improvement round may cost. NULL means unset: a round asking for more than one candidate is refused until it is set, and a single-candidate round is not capped.';
COMMENT ON COLUMN evaluation_settings.reflection_candidates_reason IS
    'Why the operator last changed reflection_candidates or reflection_round_credit_cap; NULL while neither has been changed.';
COMMENT ON COLUMN evaluation_settings.reflection_candidates_set_at IS
    'When reflection_candidates or reflection_round_credit_cap was last changed; NULL while neither has been changed.';

ALTER TABLE batch_reflections
    ADD COLUMN candidate_focuses text[] NOT NULL DEFAULT '{}'
        CONSTRAINT batch_reflections_candidate_focuses_known
        CHECK (candidate_focuses <@ ARRAY['instructions', 'scripts_and_tools', 'references_and_examples', 'free']::text[]
               AND cardinality(candidate_focuses) <> 1 AND cardinality(candidate_focuses) <= 4),
    ADD COLUMN candidate_outcomes text[] NOT NULL DEFAULT '{}'
        CONSTRAINT batch_reflections_candidate_outcomes_known
        CHECK (candidate_outcomes <@ ARRAY['unavailable', 'none_proposed', 'none_kept', 'kept']::text[]),
    ADD CONSTRAINT batch_reflections_one_outcome_per_candidate
        CHECK (cardinality(candidate_outcomes) = cardinality(candidate_focuses));

COMMENT ON COLUMN batch_reflections.candidate_focuses IS
    'The focus each candidate''s model call was given, in candidate order: instructions, scripts_and_tools, references_and_examples or free. Empty for a round of one candidate, which is asked with no focus.';
COMMENT ON COLUMN batch_reflections.candidate_outcomes IS
    'Each candidate''s own outcome, in the same order as candidate_focuses: unavailable, none_proposed, none_kept or kept. Empty for a round of one candidate, whose outcome is the round''s.';

ALTER TABLE evaluation_suggestions
    ADD COLUMN candidate smallint NOT NULL DEFAULT 0
        CONSTRAINT evaluation_suggestions_candidate_range CHECK (candidate BETWEEN 0 AND 3),
    ADD CONSTRAINT evaluation_suggestions_candidate_only_in_a_round
        CHECK (reflection_id IS NOT NULL OR candidate = 0);

COMMENT ON COLUMN evaluation_suggestions.candidate IS
    'Which candidate of its batch improvement round wrote the suggestion, counted from 0; always 0 for a suggestion on one run''s evaluation.';
