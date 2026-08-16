-- 0024_evaluation: the M3 evaluation data model (EVAL-001, EVAL-002).
-- Shape and reasoning: docs/plans/mvp/m3/evaluation-design.md §3.2.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- `evaluations` (0004) has had no writer at all - M2 executed 73 runs and
-- evaluated none of them - so every column below can be added NOT NULL without a
-- backfill. Two of them are added without a DEFAULT on purpose (`status`,
-- `evidence_complete`): they are judgements the writer has to make, and a default
-- would let a writer that forgot the field pass as though it had answered.
--
-- What this migration deliberately does NOT touch: `runs.status`, the
-- `run_status` enum, and the `runs_terminal_immutable` trigger of 0005.
-- `runs.status` answers "what happened when this executed"; `evaluations.overall`
-- answers "was the task achieved". Two questions, two columns, two tables
-- (design §4). The database already agreed: re-evaluation is append-only below,
-- so a verdict that moved a run's terminal state would be rewriting a row 0005
-- froze.

-- (a) evaluations: the columns 0004 could not have known it needed ------------
ALTER TABLE evaluations
    -- An evaluation can itself fail - judge unreachable, evidence unreadable -
    -- and that has to be a visible state rather than a row that does not exist,
    -- or the UI cannot tell "not evaluated yet" from "evaluated and it broke"
    -- (design §4.3). Distinct from `overall`: `failed` is the evaluation not
    -- running; `undetermined` is it running and honestly reaching no verdict.
    ADD COLUMN status text NOT NULL
        CHECK (status IN ('pending', 'completed', 'failed')),

    -- ADR-017: record the prompt version actually used. It is also what makes a
    -- re-evaluation comparable to the one it supersedes - "this verdict changed"
    -- is only informative next to "because the prompt/rubric changed".
    ADD COLUMN judge_prompt_version text,

    -- CONTENT-007's rubric. NULL for categories that have none; not defaulted to
    -- '' because "no rubric applies" and "rubric unrecorded" are different.
    ADD COLUMN rubric_version text,

    -- False when the trace this verdict was drawn from had holes
    -- (trace.Service's AdvancedView.complete) or an evidence ref could not be
    -- re-verified. A criterion judged on incomplete evidence must not be recorded
    -- as `passed` (丙-1, ADR-009) - the database cannot enforce that judgement,
    -- but it can refuse to let the fact go unrecorded.
    ADD COLUMN evidence_complete boolean NOT NULL,

    -- The deterministic checks of design §2.5 (spec / activation / execution /
    -- compatibility / cost), kept apart from criterion_results because they
    -- answer a different question: "what is wrong with this run" versus "did this
    -- acceptance criterion pass".
    -- [{category, severity, message, evidence: [EvidenceRef]}]
    ADD COLUMN deterministic_findings jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- What the *evaluation itself* cost. Never summed with the run's own usage
    -- (丙-3): one is what the user's workload spent, the other what the platform's
    -- verdict spent. numeric and not float, because it is money. NULL means
    -- unreported, which the UI must not render as 0 (same rule as the trace
    -- usage event's cost_usd).
    ADD COLUMN cost_usd numeric(12, 6) CHECK (cost_usd IS NULL OR cost_usd >= 0),
    -- Same vocabulary as usagePayload.cost_source in trace-event.schema.json.
    ADD COLUMN cost_source text CHECK (cost_source IN ('gateway', 'estimated')),
    -- Not a sometimes-true flag: what we record is what we saw at call time, and
    -- the authoritative figure is the gateway's per-key spend (ADR-017), so this
    -- number is structurally a floor. It is a column rather than a rendering
    -- convention for the reason contract-deltas.md §2 gives for the same field on
    -- RunComparison - carried by the data, it cannot be forgotten by a screen.
    ADD COLUMN cost_is_lower_bound boolean NOT NULL DEFAULT true,

    ADD COLUMN evaluated_at timestamptz,

    -- Set, in the same transaction, when the next evaluation of this run is
    -- written. NULL = this is the current one (see the partial unique index).
    ADD COLUMN superseded_at timestamptz;

-- A cost without its provenance cannot be labelled, and an unlabelled number is
-- exactly what NFR-001 calls misleading.
ALTER TABLE evaluations
    ADD CONSTRAINT evaluations_cost_needs_source
        CHECK ((cost_usd IS NULL) = (cost_source IS NULL));

-- Evidence refs outlive the evidence. trace_events is dropped by partition
-- (PDM-006), so a ref that carries only an event id becomes an unresolvable
-- pointer the moment retention runs, and these rows are immutable - it could
-- never be repaired. `excerpt` (the masked snapshot taken at verdict time) and
-- `available` (whether the original is still there) are what let a report say
-- "the original event is past retention, here is what it said" instead of going
-- blank or pretending (design §3.4). Written wrong once is written wrong forever,
-- so the shape is a constraint and not a comment.
ALTER TABLE evaluations
    ADD CONSTRAINT evaluations_evidence_refs_complete CHECK (
        NOT jsonb_path_exists(
            criterion_results,
            '$[*].evidence[*] ? (!exists(@.kind) || !exists(@.available) || !exists(@.excerpt))')
        AND NOT jsonb_path_exists(
            deterministic_findings,
            '$[*].evidence[*] ? (!exists(@.kind) || !exists(@.available) || !exists(@.excerpt))')
    );

-- (b) one current evaluation, all of the history -----------------------------
-- 0004 allowed exactly one evaluation per run. Re-evaluation (new rubric, new
-- judge prompt, the CONTENT-007 re-run of the 45 baseline runs) needs more than
-- one, and overwriting is the wrong answer: a verdict somebody has already read
-- and cited must not change under them. Append-only instead, with the "current"
-- one enforced by a partial unique index rather than by application discipline.
DROP INDEX evaluations_run_id_key;

CREATE UNIQUE INDEX evaluations_current_key ON evaluations (run_id)
    WHERE superseded_at IS NULL;

-- The revision list (GET /runs/{id}/evaluation/revisions) reads every evaluation
-- of a run, current or not - which the partial index above cannot serve, and
-- which nothing else indexes once the old unique index is gone.
CREATE INDEX evaluations_run_id_idx ON evaluations (run_id, created_at DESC);

-- (c) a completed verdict is a fact ------------------------------------------
-- Same shared function as 0005, same argument convention: what is listed stays
-- writable, everything else freezes, DELETE is refused.
--   feedback_helpful / feedback_comment  the user is allowed to change their
--                                        mind; EVAL-001 4 never said "once"
--   superseded_at                        stamped by the *next* evaluation
--   updated_at                           moves with the two above
-- The WHEN clause is what keeps a `pending` row writable while the job is still
-- running, and a `failed` row writable so a retry can turn it into a verdict
-- instead of leaving a corpse behind.
CREATE TRIGGER evaluations_immutable
    BEFORE UPDATE OR DELETE ON evaluations
    FOR EACH ROW WHEN (OLD.status = 'completed')
    EXECUTE FUNCTION enforce_immutable(
        'feedback_helpful', 'feedback_comment', 'superseded_at', 'updated_at');

-- (d) improvement suggestions (EVAL-002) --------------------------------------
CREATE TABLE evaluation_suggestions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  uuid NOT NULL REFERENCES workspaces (id), -- iron rule 3
    -- Hangs off one evaluation and not off the run: re-evaluating under a new
    -- rubric produces different suggestions, and the old ones have to stay
    -- attached to the verdict that produced them (design §3.2d).
    evaluation_id uuid NOT NULL REFERENCES evaluations (id),

    -- EVAL-002 2's four problem classes. `mcp` never occurs in the MVP (remote
    -- MCP is out of first release) and is a type placeholder, same treatment as
    -- TRACE-003's MCP events.
    category text NOT NULL
        CHECK (category IN ('skill', 'runtime', 'mcp', 'tool', 'dataset')),

    -- The five fields EVAL-002 1 requires. All NOT NULL: a suggestion missing
    -- any of them is not reviewable, and there is no half-suggestion worth
    -- storing.
    problem         text NOT NULL CHECK (btrim(problem) <> ''),
    -- [EvidenceRef], same shape and same reason as evaluations above.
    evidence        jsonb NOT NULL DEFAULT '[]'::jsonb,
    -- Package-relative path. The real check is in Go (normalisation, symlink
    -- targets, content hash of the file being replaced - design §5.2); this is
    -- the floor underneath it, so an escaping path cannot even be stored while
    -- somebody is deciding whether to apply it.
    target_path     text NOT NULL
        CHECK (target_path <> ''
               AND left(target_path, 1) <> '/'
               AND target_path !~ '(^|/)\.\.(/|$)'),
    proposed_change text NOT NULL,
    expected_impact text NOT NULL,

    -- EVAL-002 3: accepted or rejected one at a time. Recording the decision and
    -- applying it are separate on purpose - five accepted suggestions become one
    -- new version, not five (design §5.3).
    decision    text NOT NULL DEFAULT 'pending'
        CHECK (decision IN ('pending', 'accepted', 'rejected')),
    decided_at  timestamptz,
    -- EVAL-002 4: points at the *new* version created from this suggestion. The
    -- old version is not touched (iron rule 4). NULL = not applied yet.
    applied_skill_version_id uuid REFERENCES skill_versions (id),
    created_at  timestamptz NOT NULL DEFAULT now(),

    -- A decision with no timestamp cannot be audited, and a timestamp with no
    -- decision is a decision nobody made.
    CONSTRAINT evaluation_suggestions_decision_dated
        CHECK ((decision = 'pending') = (decided_at IS NULL)),
    -- Nothing gets applied that was not accepted.
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

-- The model's proposal is a fact about what the model proposed; only the human
-- decision on it, and the version that decision produced, are still open.
CREATE TRIGGER evaluation_suggestions_immutable
    BEFORE UPDATE OR DELETE ON evaluation_suggestions
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable(
        'decision', 'decided_at', 'applied_skill_version_id');
