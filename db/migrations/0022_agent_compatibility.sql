-- 0022_agent_compatibility: the DISC-002「Agent 相容」axis, which until now had
-- no column to be written to and was therefore a constant `unverified` in Go
-- (catalog/http.go resultFacets, catalog/detail.go compatibility).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- Why the key is (skill_version_id, runtime_image) and not skill_version_id
-- alone — the four open questions in m2/content-baseline-report.md §8, answered:
--
--  1. Ownership. A version-level verdict would be a lie the moment the image
--     changes. The M2 baseline's finding that 33 skills could not run their own
--     scripts is a fact about `skillhub/runtime-agent-sdk:2026.08-1`, which had
--     no python3; the very next image adds it, and the same package on the same
--     version gets a different answer. The measurement is of a pair, so the row
--     is keyed by the pair. The practical consequence is deliberate: bumping the
--     runtime image drops every skill back to "no row" — i.e. unverified — until
--     a baseline re-run measures the new pair. Nothing else is honest, and a
--     stale verdict carried across an image change is precisely what would make
--     the filter worse than not having one.
--
--  2. Judgements per axis. `capability` answers "does the agent actually pick
--     this skill up when it is mounted" and comes straight from the trace: a
--     `skill_activation` event for that run or nothing. `runtime` answers "can
--     the package's own scripts execute in this image", i.e. whether the runtime
--     the scripts are written for is one the image provides.
--
--  3. Sample size. One baseline Run is enough to write a row, because the row
--     says what one Run observed and carries `source_run_id` to prove it. The
--     alternative — hold the field empty until some quorum exists — leaves the
--     catalogue saying `unverified` while 45 real measurements sit unused, which
--     is the state this table exists to end. A later re-measurement overwrites
--     the row for the same pair and moves `source_run_id` with it.
--
--  4. Value range. `runtime` needs a third state beyond passed/failed, because
--     what the baseline actually observed is neither: the scripts were never
--     executed and the model re-implemented them instead. That is `transpiled` —
--     a working Run whose work was not the Skill's own code. Collapsing it into
--     `native` claims an execution that did not happen; collapsing it into
--     `failed` contradicts the artifacts the Run produced.
--
-- Not a column on search_documents: that is a projection rebuilt from the source
-- of truth (ADR-010), and a measurement is not derivable from the package. It is
-- also not on skill_versions, which is immutable (iron rule 4) — a measurement
-- arrives after the version and can be repeated.

CREATE TABLE skill_runtime_compatibility (
    -- The catalog version, not the fork that was actually executed. The baseline
    -- forks a catalog skill into a scratch workspace (WS-001, content-addressed:
    -- identical bytes), runs the fork, and the lineage runs back through
    -- skills.forked_from_version_id. Recording the fork would key the result to a
    -- throwaway row nobody can look up from the catalogue.
    skill_version_id uuid NOT NULL REFERENCES skill_versions (id) ON DELETE CASCADE,

    -- Full image reference. A digest reference is the right form (I-02, and it is
    -- what SKILLHUB_SANDBOX_IMAGE must carry in production) and is what rows
    -- written after SBX-011 will hold. The M2 baseline predates the GHCR
    -- publishing pipeline and its image was never pushed, so it has no registry
    -- digest at all; its row records the tag it genuinely ran under rather than a
    -- digest invented to satisfy the column's preferred shape.
    runtime_image    text NOT NULL CHECK (runtime_image <> ''),

    -- Does the agent activate the skill when it is mounted.
    --   activated      — a skill_activation event for this skill in the run trace
    --   not_activated  — the run completed and the trace shows the skill was
    --                    offered and something else was used instead
    --   unverified     — no run, or a run that ended before the question could be
    --                    answered. Never inferred from absence: the SDK message
    --                    stream cannot show "available but not called" at all
    --                    (TRACE-002 限制註記), so a missing activation event on a
    --                    truncated run is not evidence of non-activation.
    capability       text NOT NULL
        CHECK (capability IN ('activated', 'not_activated', 'unverified')),

    -- Can the package's own scripts run in this image.
    --   native      — the runtimes the package declares are all provided here
    --   transpiled  — they are not, and the observed Run got its result from the
    --                 model re-implementing the script rather than running it
    --   failed      — they are not, and the Run failed because of it
    --   unverified  — not measured on this pair
    runtime          text NOT NULL
        CHECK (runtime IN ('native', 'transpiled', 'failed', 'unverified')),

    -- The Run this row is derived from, so any displayed verdict can be walked
    -- back to a trace, an artifact manifest and a Test Case snapshot
    -- (CONTENT-008 允收第 3 條). NULL only for a row asserted without a Run,
    -- which nothing currently writes.
    source_run_id    uuid REFERENCES runs (id),
    measured_at      timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (skill_version_id, runtime_image)
);

-- The read path is "newest measurement for this version, whichever image it was
-- on" (catalog/detail.go, db/queries/search.sql): the answer is labelled with the
-- image it came from rather than filtered to a configured one, so there is no
-- deployment setting that can silently decide which verdict the catalogue shows.
CREATE INDEX skill_runtime_compatibility_latest_idx
    ON skill_runtime_compatibility (skill_version_id, measured_at DESC);
