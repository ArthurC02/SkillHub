ALTER TABLE skill_versions
    ADD COLUMN held_for_evaluation boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN skill_versions.held_for_evaluation IS
    'True for a version built only to be evaluated as a candidate of an improvement round: it runs, forks and downloads by its own id, but it is outside the Skill''s line, so it is never the newest version, never released and never indexed, until it is promoted.';

CREATE TABLE skill_version_promotions (
    version_id   uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE NO ACTION,
    promoted_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    promoted_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT skill_version_promotions_version_fkey
        FOREIGN KEY (version_id, workspace_id) REFERENCES skill_versions (id, workspace_id) ON DELETE CASCADE
);

CREATE FUNCTION refuse_promoting_a_version_not_held() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM skill_versions WHERE id = NEW.version_id AND held_for_evaluation) THEN
        RAISE EXCEPTION 'version % was not held for evaluation, so it is already in its Skill''s line', NEW.version_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END
$$;

COMMENT ON FUNCTION refuse_promoting_a_version_not_held() IS
    'Refuses a promotion row for a version that was not held for evaluation, since only a held version can be brought into its Skill''s line.';

CREATE TRIGGER skill_version_promotions_only_held
    BEFORE INSERT ON skill_version_promotions
    FOR EACH ROW EXECUTE FUNCTION refuse_promoting_a_version_not_held();

CREATE TRIGGER skill_version_promotions_immutable
    BEFORE UPDATE OR DELETE ON skill_version_promotions
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

COMMENT ON TABLE skill_version_promotions IS
    'The held versions brought into their Skill''s line by adoption; written once, so a promoted version stays in the line.';
COMMENT ON COLUMN skill_version_promotions.version_id IS
    'The held version that was promoted.';
COMMENT ON COLUMN skill_version_promotions.workspace_id IS
    'Workspace that owns the version.';
COMMENT ON COLUMN skill_version_promotions.promoted_by IS
    'The user whose adoption promoted it; cleared when that user is deleted.';
COMMENT ON COLUMN skill_version_promotions.promoted_at IS
    'When the version entered the line.';

CREATE VIEW skill_line_versions AS
SELECT v.* FROM skill_versions v
WHERE NOT v.held_for_evaluation
   OR EXISTS (SELECT 1 FROM skill_version_promotions p WHERE p.version_id = v.id);

COMMENT ON VIEW skill_line_versions IS
    'The Skill''s line: its versions that were not held for evaluation, plus the held ones that were promoted. Every read of a Skill''s newest, latest or previous version reads this view, so a candidate never becomes the face of its Skill before it is adopted.';

ALTER TABLE evaluation_batches
    ADD COLUMN origin_key text
        CONSTRAINT evaluation_batches_origin_key_present CHECK (origin_key IS NULL OR btrim(origin_key) <> '');

CREATE UNIQUE INDEX evaluation_batches_origin_key_idx
    ON evaluation_batches (workspace_id, origin_key) WHERE origin_key IS NOT NULL;

COMMENT ON COLUMN evaluation_batches.origin_key IS
    'The idempotency key of the platform action that launched the batch, so a repeated launch returns the batch it already made; NULL for a batch a user started by hand.';

ALTER TABLE evaluation_suggestions
    ADD COLUMN decided_by uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD CONSTRAINT evaluation_suggestions_decider_only_when_decided
        CHECK (decided_by IS NULL OR decision <> 'pending');

COMMENT ON COLUMN evaluation_suggestions.decided_by IS
    'The user whose action recorded the decision; NULL while pending, for a decision recorded before this was kept, or after that user was deleted.';

DROP TRIGGER evaluation_suggestions_immutable ON evaluation_suggestions;

CREATE TRIGGER evaluation_suggestions_immutable
    BEFORE UPDATE OR DELETE ON evaluation_suggestions
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable(
        'decision', 'decided_at', 'decided_by', 'applied_skill_version_id');

CREATE TABLE reflection_candidate_batches (
    workspace_id           uuid NOT NULL REFERENCES workspaces (id) ON DELETE NO ACTION,
    reflection_id          uuid NOT NULL,
    candidate              smallint NOT NULL CHECK (candidate BETWEEN 0 AND 3),
    skill_version_id       uuid,
    batch_id               uuid,
    skipped_suggestion_ids uuid[] NOT NULL DEFAULT '{}',
    created_by             uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at             timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (reflection_id, candidate),
    CONSTRAINT reflection_candidate_batches_reflection_fkey
        FOREIGN KEY (reflection_id, workspace_id) REFERENCES batch_reflections (id, workspace_id) ON DELETE CASCADE,
    CONSTRAINT reflection_candidate_batches_batch_fkey
        FOREIGN KEY (batch_id, workspace_id, skill_version_id)
        REFERENCES evaluation_batches (id, workspace_id, skill_version_id) ON DELETE CASCADE,
    CONSTRAINT reflection_candidate_batches_built_or_unavailable
        CHECK (num_nonnulls(skill_version_id, batch_id) IN (0, 2)),
    CONSTRAINT reflection_candidate_batches_one_batch UNIQUE (batch_id),
    CONSTRAINT reflection_candidate_batches_version_key UNIQUE (reflection_id, candidate, skill_version_id)
);

CREATE TRIGGER reflection_candidate_batches_immutable
    BEFORE UPDATE OR DELETE ON reflection_candidate_batches
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

COMMENT ON TABLE reflection_candidate_batches IS
    'The version built from one candidate of a batch improvement round and the batch that ran it on all of the round''s test cases; written once when the candidates were evaluated.';
COMMENT ON COLUMN reflection_candidate_batches.workspace_id IS
    'Workspace that owns the round and therefore this candidate''s evaluation.';
COMMENT ON COLUMN reflection_candidate_batches.reflection_id IS
    'The improvement round whose candidate this is.';
COMMENT ON COLUMN reflection_candidate_batches.candidate IS
    'Which candidate of the round, counted from 0; always below the number of candidates the round asked for.';
COMMENT ON COLUMN reflection_candidate_batches.skill_version_id IS
    'The version built from the candidate''s applicable suggestions; NULL with batch_id when none applied, so the candidate is unavailable rather than an empty version.';
COMMENT ON COLUMN reflection_candidate_batches.batch_id IS
    'The batch that ran the candidate''s version on the round''s test cases with the source batch''s trials and arms; NULL with skill_version_id when the candidate is unavailable.';
COMMENT ON COLUMN reflection_candidate_batches.skipped_suggestion_ids IS
    'The candidate''s suggestions left out of its version: those the user had rejected and those that failed their re-check against the package.';
COMMENT ON COLUMN reflection_candidate_batches.created_by IS
    'The user who asked for the candidates to be evaluated; cleared when that user is deleted.';
COMMENT ON COLUMN reflection_candidate_batches.created_at IS
    'When the candidate''s batch was launched or the candidate was found unavailable.';

CREATE TABLE reflection_adoptions (
    reflection_id uuid PRIMARY KEY,
    workspace_id  uuid NOT NULL REFERENCES workspaces (id) ON DELETE NO ACTION,
    candidate     smallint NOT NULL,
    skill_version_id uuid NOT NULL,
    adopted_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    adopted_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT reflection_adoptions_candidate_fkey
        FOREIGN KEY (reflection_id, candidate, skill_version_id)
        REFERENCES reflection_candidate_batches (reflection_id, candidate, skill_version_id) ON DELETE CASCADE
);

CREATE TRIGGER reflection_adoptions_immutable
    BEFORE UPDATE OR DELETE ON reflection_adoptions
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

COMMENT ON TABLE reflection_adoptions IS
    'The one candidate of a batch improvement round the user adopted; written once, so the line of versions a next round may start from never forks.';
COMMENT ON COLUMN reflection_adoptions.reflection_id IS
    'The improvement round whose candidate was adopted.';
COMMENT ON COLUMN reflection_adoptions.workspace_id IS
    'Workspace that owns the round.';
COMMENT ON COLUMN reflection_adoptions.candidate IS
    'The adopted candidate, counted from 0.';
COMMENT ON COLUMN reflection_adoptions.skill_version_id IS
    'The adopted candidate''s built version; NOT NULL, so an unavailable candidate, which has none, can never be adopted.';
COMMENT ON COLUMN reflection_adoptions.adopted_by IS
    'The user who adopted it; cleared when that user is deleted.';
COMMENT ON COLUMN reflection_adoptions.adopted_at IS
    'When the candidate was adopted.';

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
        'skill.restored',
        'skill.version_promoted'
    ));
