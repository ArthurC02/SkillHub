COMMENT ON COLUMN cost_statistics.kind IS
    'Kind of model call or session these percentiles describe. The call kinds match cost_events.kind: creation_step, search_embedding, index_enrich, review, suggestion, generate, run, match_reasons, suggest_criteria, search_intent and platform_agent; creation_session is the extra kind for a whole interactive creation session, so a new session can be refused before it starts when the balance cannot cover one.';

COMMENT ON TYPE run_status IS
    'Lifecycle position of a run, in order. queued: accepted and waiting for a worker to pick a provider; provisioning: a provider was chosen and asked for a sandbox; preparing: the provider accepted the dispatch and is building the sandbox, from which point the run counts against the workspace quota; running: the skill is executing in the sandbox; evaluating: the output is being judged against the test cases; succeeded, failed, cancelled and timed_out are final and never left.';

COMMENT ON TYPE run_cleanup_status IS
    'How far releasing a run''s execution resources has got. pending: nothing released yet; cleaning_up: the worker is revoking the attempts'' model gateway keys, destroying their sandboxes and charging the run''s cost; cleaned: every step completed; failed: at least one step did not complete and the reasons are recorded with the run. cleaned and failed are settled and never left.';
