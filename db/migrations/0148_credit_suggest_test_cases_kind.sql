ALTER TABLE cost_events DROP CONSTRAINT cost_events_kind_check;
ALTER TABLE cost_events ADD CONSTRAINT cost_events_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run', 'match_reasons', 'suggest_criteria',
    'search_intent', 'platform_agent', 'injection_check', 'suggest_test_cases'));

ALTER TABLE cost_statistics DROP CONSTRAINT cost_statistics_kind_check;
ALTER TABLE cost_statistics ADD CONSTRAINT cost_statistics_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run', 'creation_session', 'match_reasons',
    'suggest_criteria', 'search_intent', 'platform_agent', 'injection_check', 'suggest_test_cases'));

COMMENT ON COLUMN cost_events.kind IS
    'Which feature made the paid call: creation_step (one step of an interactive creation session), search_embedding (embedding a search query), index_enrich (enriching a skill''s catalog metadata), review (an AI review), injection_check (asking whether a run''s output addresses its reviewer, priced by the operator), suggestion (an improvement suggestion), generate (generating a skill from a description), run (a trial run), match_reasons, suggest_criteria, search_intent (search-time model calls), platform_agent (a platform agent step) or suggest_test_cases (proposing test cases for a skill).';

COMMENT ON COLUMN cost_statistics.kind IS
    'Kind of model call or session these percentiles describe. The call kinds match cost_events.kind: creation_step, search_embedding, index_enrich, review, injection_check, suggestion, generate, run, match_reasons, suggest_criteria, search_intent, platform_agent and suggest_test_cases; creation_session is the extra kind for a whole interactive creation session, so a new session can be refused before it starts when the balance cannot cover one.';

COMMENT ON COLUMN model_call_budgets.kind IS
    'The model endpoint the ceiling applies to: one of enrich-skill, generate-skill, analyze-intent, match-reasons, suggest-criteria, suggest-test-cases, judge-run or suggest-improvements.';
