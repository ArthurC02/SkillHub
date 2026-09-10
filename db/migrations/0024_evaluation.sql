ALTER TABLE evaluations
    ADD COLUMN status text NOT NULL
        CHECK (status IN ('pending', 'completed', 'failed')),

    ADD COLUMN judge_prompt_version text,

    ADD COLUMN rubric_version text,

    ADD COLUMN evidence_complete boolean NOT NULL,

    ADD COLUMN deterministic_findings jsonb NOT NULL DEFAULT '[]'::jsonb,

    ADD COLUMN cost_usd numeric(12, 6) CHECK (cost_usd IS NULL OR cost_usd >= 0),
    ADD COLUMN cost_source text CHECK (cost_source IN ('gateway', 'estimated')),
    ADD COLUMN cost_is_lower_bound boolean NOT NULL DEFAULT true,

    ADD COLUMN evaluated_at timestamptz,

    ADD COLUMN superseded_at timestamptz;

ALTER TABLE evaluations
    ADD CONSTRAINT evaluations_cost_needs_source
        CHECK ((cost_usd IS NULL) = (cost_source IS NULL));

ALTER TABLE evaluations
    ADD CONSTRAINT evaluations_evidence_refs_complete CHECK (
        NOT jsonb_path_exists(
            criterion_results,
            '$[*].evidence[*] ? (!exists(@.kind) || !exists(@.available) || !exists(@.excerpt))')
        AND NOT jsonb_path_exists(
            deterministic_findings,
            '$[*].evidence[*] ? (!exists(@.kind) || !exists(@.available) || !exists(@.excerpt))')
    );

DROP INDEX evaluations_run_id_key;

CREATE UNIQUE INDEX evaluations_current_key ON evaluations (run_id)
    WHERE superseded_at IS NULL;

CREATE INDEX evaluations_run_id_idx ON evaluations (run_id, created_at DESC);

CREATE TRIGGER evaluations_immutable
    BEFORE UPDATE OR DELETE ON evaluations
    FOR EACH ROW WHEN (OLD.status = 'completed')
    EXECUTE FUNCTION enforce_immutable(
        'feedback_helpful', 'feedback_comment', 'superseded_at', 'updated_at');

CREATE TABLE evaluation_suggestions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  uuid NOT NULL REFERENCES workspaces (id), -- iron rule 3
    evaluation_id uuid NOT NULL REFERENCES evaluations (id),

    category text NOT NULL
        CHECK (category IN ('skill', 'runtime', 'mcp', 'tool', 'dataset')),

    problem         text NOT NULL CHECK (btrim(problem) <> ''),
    evidence        jsonb NOT NULL DEFAULT '[]'::jsonb,
    target_path     text NOT NULL
        CHECK (target_path <> ''
               AND left(target_path, 1) <> '/'
               AND target_path !~ '(^|/)\.\.(/|$)'),
    proposed_change text NOT NULL,
    expected_impact text NOT NULL,

    decision    text NOT NULL DEFAULT 'pending'
        CHECK (decision IN ('pending', 'accepted', 'rejected')),
    decided_at  timestamptz,
    applied_skill_version_id uuid REFERENCES skill_versions (id),
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT evaluation_suggestions_decision_dated
        CHECK ((decision = 'pending') = (decided_at IS NULL)),
    CONSTRAINT evaluation_suggestions_applied_needs_accept
        CHECK (applied_skill_version_id IS NULL OR decision = 'accepted'),
    CONSTRAINT evaluation_suggestions_evidence_refs_complete CHECK (
        NOT jsonb_path_exists(
            evidence,
            '$[*] ? (!exists(@.kind) || !exists(@.available) || !exists(@.excerpt))')
    )
);

CREATE INDEX evaluation_suggestions_workspace_id_idx
    ON evaluation_suggestions (workspace_id);
CREATE INDEX evaluation_suggestions_evaluation_id_idx
    ON evaluation_suggestions (evaluation_id, created_at);

CREATE TRIGGER evaluation_suggestions_immutable
    BEFORE UPDATE OR DELETE ON evaluation_suggestions
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable(
        'decision', 'decided_at', 'applied_skill_version_id');
