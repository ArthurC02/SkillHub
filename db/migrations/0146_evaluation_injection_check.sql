ALTER TABLE evaluations
    ADD COLUMN injection_check text
        CONSTRAINT evaluations_injection_check_known
        CHECK (injection_check IS NULL OR injection_check IN ('flagged', 'clear', 'unchecked')),
    ADD COLUMN injection_probability double precision
        CONSTRAINT evaluations_injection_probability_range
        CHECK (injection_probability IS NULL OR injection_probability BETWEEN 0 AND 1),
    ADD CONSTRAINT evaluations_injection_probability_answers_the_check
        CHECK (CASE WHEN injection_check IN ('flagged', 'clear') THEN injection_probability IS NOT NULL
                    ELSE injection_probability IS NULL END);

COMMENT ON COLUMN evaluations.injection_check IS
    'Whether the run''s output, artifacts and trace were checked for text addressed to the evaluator instead of the user, written once with the verdict: flagged (the check answered at or above the line, and the verdict was still decided as usual), clear (the check answered below the line), unchecked (the check was due but did not answer, so nothing is known). NULL when the check was switched off for this evaluation or the evaluation predates the check.';

COMMENT ON COLUMN evaluations.injection_probability IS
    'The probability the injection check answered, between 0 and 1. Present exactly when injection_check is flagged or clear.';

ALTER TABLE evaluation_model_usage
    DROP CONSTRAINT evaluation_model_usage_operation_check,
    ADD CONSTRAINT evaluation_model_usage_operation_check
    CHECK (operation IN ('judge', 'suggest', 'injection_check'));

COMMENT ON COLUMN evaluation_model_usage.operation IS
    'Which step of the evaluation made the call. judge: deciding the verdict. suggest: writing improvement suggestions. injection_check: asking whether the run''s output addresses the evaluator. Each evaluation has at most one row per operation.';

ALTER TABLE evaluation_settings
    ALTER COLUMN reason DROP NOT NULL,
    ALTER COLUMN set_at DROP NOT NULL,
    ALTER COLUMN set_at DROP DEFAULT,
    ADD COLUMN injection_check boolean NOT NULL DEFAULT false,
    ADD COLUMN injection_check_credits_per_million_tokens integer
        CONSTRAINT evaluation_settings_injection_check_price_positive
        CHECK (injection_check_credits_per_million_tokens IS NULL OR injection_check_credits_per_million_tokens > 0),
    ADD COLUMN injection_check_reason text
        CONSTRAINT evaluation_settings_injection_check_reason_check
        CHECK (injection_check_reason IS NULL OR btrim(injection_check_reason) <> ''),
    ADD COLUMN injection_check_set_at timestamptz,
    ADD CONSTRAINT evaluation_settings_injection_check_needs_a_price
        CHECK (NOT injection_check OR injection_check_credits_per_million_tokens IS NOT NULL),
    ADD CONSTRAINT evaluation_settings_injection_check_change_is_explained
        CHECK ((injection_check_reason IS NULL) = (injection_check_set_at IS NULL)),
    ADD CONSTRAINT evaluation_settings_judge_panel_change_is_explained
        CHECK ((reason IS NULL) = (set_at IS NULL));

COMMENT ON TABLE evaluation_settings IS
    'Operator-set evaluation policy. An absent row means every setting is at its default: one judge, no panel, no injection check.';

COMMENT ON COLUMN evaluation_settings.reason IS
    'Why the operator last changed judge_panel. NULL while judge_panel has never been changed, even when the row exists for another setting.';

COMMENT ON COLUMN evaluation_settings.set_by IS
    'The operator who last changed judge_panel; NULL while it has never been changed, or once that account is removed.';

COMMENT ON COLUMN evaluation_settings.set_at IS
    'When judge_panel was last changed; NULL while it has never been changed.';

COMMENT ON COLUMN evaluation_settings.injection_check IS
    'Whether every judged evaluation also asks whether the run''s output addresses the evaluator. Off by default; the verdict is decided the same way either way.';

COMMENT ON COLUMN evaluation_settings.injection_check_credits_per_million_tokens IS
    'What the operator says one million input tokens of the injection check cost, in credits. The gateway records these calls as free, so this is the only price the platform has for them. Required while the check is on; kept when it is switched off.';

COMMENT ON COLUMN evaluation_settings.injection_check_reason IS
    'Why the operator last changed the injection check. NULL while it has never been changed. Who changed it is in the audit log.';

COMMENT ON COLUMN evaluation_settings.injection_check_set_at IS
    'When the injection check was last changed; NULL while it has never been changed.';

ALTER TABLE cost_events DROP CONSTRAINT cost_events_kind_check;
ALTER TABLE cost_events ADD CONSTRAINT cost_events_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run', 'match_reasons', 'suggest_criteria',
    'search_intent', 'platform_agent', 'injection_check'));

ALTER TABLE cost_statistics DROP CONSTRAINT cost_statistics_kind_check;
ALTER TABLE cost_statistics ADD CONSTRAINT cost_statistics_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run', 'creation_session', 'match_reasons',
    'suggest_criteria', 'search_intent', 'platform_agent', 'injection_check'));

COMMENT ON COLUMN cost_events.kind IS
    'Which feature made the paid call: creation_step (one step of an interactive creation session), search_embedding (embedding a search query), index_enrich (enriching a skill''s catalog metadata), review (an AI review), injection_check (asking whether a run''s output addresses its reviewer, priced by the operator), suggestion (an improvement suggestion), generate (generating a skill from a description), run (a trial run), match_reasons, suggest_criteria, search_intent (search-time model calls) or platform_agent (a platform agent step).';

COMMENT ON COLUMN cost_statistics.kind IS
    'Kind of model call or session these percentiles describe. The call kinds match cost_events.kind: creation_step, search_embedding, index_enrich, review, injection_check, suggestion, generate, run, match_reasons, suggest_criteria, search_intent and platform_agent; creation_session is the extra kind for a whole interactive creation session, so a new session can be refused before it starts when the balance cannot cover one.';
