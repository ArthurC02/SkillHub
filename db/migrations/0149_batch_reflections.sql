ALTER TABLE evaluation_batches
    ADD CONSTRAINT evaluation_batches_id_workspace_version_key UNIQUE (id, workspace_id, skill_version_id);

COMMENT ON CONSTRAINT evaluation_batches_id_workspace_version_key ON evaluation_batches IS
    'Exists only as the target of batch_reflections_batch_fkey, so a round''s copy of the batch''s skill version can never disagree with the batch; id alone is already the primary key.';

CREATE TABLE batch_reflections (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id      uuid NOT NULL REFERENCES workspaces (id) ON DELETE NO ACTION,
    batch_id          uuid NOT NULL,
    skill_version_id  uuid NOT NULL,
    dev_test_case_ids uuid[] NOT NULL CHECK (cardinality(dev_test_case_ids) >= 1),
    outcome           text NOT NULL
        CHECK (outcome IN ('restricted', 'not_needed', 'no_evidence', 'unavailable', 'none_proposed', 'none_kept', 'kept')),
    runs_read         integer NOT NULL CHECK (runs_read >= 0),
    runs_not_read     integer NOT NULL CHECK (runs_not_read >= 0),
    created_by        uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT batch_reflections_id_workspace_key UNIQUE (id, workspace_id),
    CONSTRAINT batch_reflections_batch_fkey
        FOREIGN KEY (batch_id, workspace_id, skill_version_id)
        REFERENCES evaluation_batches (id, workspace_id, skill_version_id) ON DELETE CASCADE
);

CREATE INDEX batch_reflections_batch_idx ON batch_reflections (workspace_id, batch_id, created_at DESC);

CREATE TRIGGER batch_reflections_immutable
    BEFORE UPDATE OR DELETE ON batch_reflections
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

COMMENT ON TABLE batch_reflections IS
    'One improvement round read from the failures of an evaluation batch''s development test cases; the batch''s other test cases are held out and never read.';
COMMENT ON COLUMN batch_reflections.workspace_id IS
    'Workspace that owns the batch and therefore this round.';
COMMENT ON COLUMN batch_reflections.batch_id IS
    'The evaluation batch whose runs were read.';
COMMENT ON COLUMN batch_reflections.skill_version_id IS
    'The skill version every read run ran, and so the version the suggestions were written against; the foreign key keeps it equal to the batch''s own version.';
COMMENT ON COLUMN batch_reflections.dev_test_case_ids IS
    'The batch test cases whose runs the model was shown, frozen when the round was made. Every other test case of the batch is held out for checking the result.';
COMMENT ON COLUMN batch_reflections.outcome IS
    'Why the round holds the suggestions it holds: restricted (licence hold, no model call), not_needed (every development run passed with nothing to improve, no model call), no_evidence (no failing run cited anything), unavailable (the model call failed, which says nothing about the skill), none_proposed, none_kept (proposals failed the evidence or storage checks) or kept.';
COMMENT ON COLUMN batch_reflections.runs_read IS
    'How many failing development runs were put in front of the model.';
COMMENT ON COLUMN batch_reflections.runs_not_read IS
    'How many failing development runs were left out to stay within the request size, so a partial reading is never presented as a complete one.';
COMMENT ON COLUMN batch_reflections.created_by IS
    'The user who asked for the round; cleared when that user is deleted.';
COMMENT ON COLUMN batch_reflections.created_at IS
    'When the round was recorded, after the model answered.';

ALTER TABLE evaluation_suggestions
    ALTER COLUMN evaluation_id DROP NOT NULL,
    ADD COLUMN reflection_id uuid,
    ADD CONSTRAINT evaluation_suggestions_reflection_id_fkey
        FOREIGN KEY (reflection_id, workspace_id) REFERENCES batch_reflections (id, workspace_id) ON DELETE NO ACTION,
    ADD CONSTRAINT evaluation_suggestions_one_owner
        CHECK (num_nonnulls(evaluation_id, reflection_id) = 1);

CREATE INDEX evaluation_suggestions_reflection_id_idx
    ON evaluation_suggestions (reflection_id, created_at) WHERE reflection_id IS NOT NULL;

COMMENT ON COLUMN evaluation_suggestions.evaluation_id IS
    'The evaluation of one run that produced this suggestion. Exactly one of evaluation_id and reflection_id is set.';
COMMENT ON COLUMN evaluation_suggestions.reflection_id IS
    'The batch improvement round that produced this suggestion. Exactly one of evaluation_id and reflection_id is set.';

ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_event_type_check;

ALTER TABLE outbox_events
    ADD CONSTRAINT outbox_events_event_type_check CHECK (event_type IN (
        'run.queued',
        'run.provisioning',
        'run.preparing',
        'run.running',
        'run.evaluating',
        'run.succeeded',
        'run.failed',
        'run.cancelled',
        'run.timed_out',
        'run.cleanup_cleaned',
        'run.cleanup_failed',
        'run.cancel_requested',
        'run.provider_assigned',
        'run.attempt_started',
        'run.attempt_dispatched',
        'run.attempt_finished',
        'run.object_grants_recorded',
        'evaluation.started',
        'evaluation.superseded',
        'evaluation.completed',
        'evaluation.failed',
        'evaluation.feedback_recorded',
        'evaluation.suggestion_decided',
        'evaluation.suggestions_applied',
        'batch_reflection.suggestion_decided',
        'batch_reflection.suggestions_applied',
        'skill.taken_down',
        'skill.access_restricted',
        'skill.access_restriction_lifted',
        'skill.redistribution_set',
        'skill.categorized',
        'skill.deleted',
        'skill.created',
        'skill.version_added',
        'skill.described',
        'skill.curation_set',
        'skill.restored'
    ));

ALTER TABLE cost_events DROP CONSTRAINT cost_events_kind_check;
ALTER TABLE cost_events ADD CONSTRAINT cost_events_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run', 'match_reasons', 'suggest_criteria',
    'search_intent', 'platform_agent', 'injection_check', 'suggest_test_cases', 'batch_reflection'));

ALTER TABLE cost_statistics DROP CONSTRAINT cost_statistics_kind_check;
ALTER TABLE cost_statistics ADD CONSTRAINT cost_statistics_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run', 'creation_session', 'match_reasons',
    'suggest_criteria', 'search_intent', 'platform_agent', 'injection_check', 'suggest_test_cases',
    'batch_reflection'));

COMMENT ON COLUMN cost_events.kind IS
    'Which feature made the paid call: creation_step (one step of an interactive creation session), search_embedding (embedding a search query), index_enrich (enriching a skill''s catalog metadata), review (an AI review), injection_check (asking whether a run''s output addresses its reviewer, priced by the operator), suggestion (an improvement suggestion), batch_reflection (one improvement round read from an evaluation batch), generate (generating a skill from a description), run (a trial run), match_reasons, suggest_criteria, search_intent (search-time model calls), platform_agent (a platform agent step) or suggest_test_cases (proposing test cases for a skill).';

COMMENT ON COLUMN cost_statistics.kind IS
    'Kind of model call or session these percentiles describe. The call kinds match cost_events.kind: creation_step, search_embedding, index_enrich, review, injection_check, suggestion, batch_reflection, generate, run, match_reasons, suggest_criteria, search_intent, platform_agent and suggest_test_cases; creation_session is the extra kind for a whole interactive creation session, so a new session can be refused before it starts when the balance cannot cover one.';
