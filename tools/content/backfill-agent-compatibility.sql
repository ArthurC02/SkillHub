-- Backfill: the M2 baseline's Agent-compatibility measurements (0022).
--
-- Source of the numbers: plans/mvp/m2/content-baseline-report.md §4, §8 and §12.
--
-- One invocation backfills one (image, run window) pair, because 0022 keys a
-- measurement by (skill version, runtime image) and a Run's image is not
-- recorded anywhere the control plane can read: the image is execution-plane
-- configuration (SKILLHUB_SANDBOX_IMAGE on the node), and the RunResult contract
-- does not carry it back. So the caller states which image was configured and
-- which Runs were made under it, and the window is what keeps the two batches
-- apart. Without it, re-running after the 2026.08-2 batch would relabel those
-- nine measurements as 2026.08-1 — the newest Run per version is now on the new
-- image — and quietly corrupt the 36 rows it was supposed to leave alone.
--
-- This is data, not schema, so it is not in the migration: a fresh database has
-- no skills, no runs and no traces, and a migration that inserts measurements
-- nobody made would be a fixture pretending to be a fact. It is re-runnable
-- (ON CONFLICT updates in place) and derives everything it can from the database
-- rather than restating it:
--
--   * capability comes from the trace. `activated` iff the Run this row cites
--     emitted a skill_activation event; anything else is `unverified`, never
--     `not_activated` — the SDK message stream cannot show "offered and not
--     used" (TRACE-002 限制註記), so silence is not evidence of refusal.
--   * runtime comes from the rule, applied to data: a package whose scripts are
--     written for a runtime the image does not provide could not execute them,
--     and the Run's result came from the model re-implementing them —
--     `transpiled`. When the image does provide it, the scripts are the thing
--     that ran — `native`. Which of the two a Python package gets is therefore a
--     property of the image, so it is the `python_runtime` variable rather than
--     a constant: 2026.08-1 had no python3 (`transpiled`), 2026.08-2 ships it
--     (`native`). `failed` is not used here: every Python skill measured so far
--     still produced a result.
--   * source_run_id and measured_at come from the newest Run per forked-from
--     version *within the window*, which is the Run the report's tables record.
--
-- The only literal data is the declared runtime per skill, mirroring
-- `deps_runtime` in tools/content/seed-skills.json. It is spelled out rather
-- than re-derived from the package because it is a curation judgement about what
-- the SKILL.md's worked examples are written for, and that judgement lives in
-- the seed list.
--
-- The two batches measured so far, each re-derivable on its own:
--
--   # 45 skills on 2026.08-1 (no python3), the original M2 baseline
--   psql -v ON_ERROR_STOP=1 --single-transaction \
--        -v until='2026-08-16 10:00:00+00' \
--        -f tools/content/backfill-agent-compatibility.sql
--
--   # the 9 re-run on 2026.08-2 (ships python3) after the budget fix
--   psql -v ON_ERROR_STOP=1 --single-transaction \
--        -v image=skillhub/runtime-agent-sdk:2026.08-2 \
--        -v python_runtime=native \
--        -v since='2026-08-16 10:00:00+00' \
--        -f tools/content/backfill-agent-compatibility.sql

\if :{?image}
\else
\set image 'skillhub/runtime-agent-sdk:2026.08-1'
\endif
\if :{?python_runtime}
\else
\set python_runtime 'transpiled'
\endif
\if :{?since}
\else
\set since '-infinity'
\endif
\if :{?until}
\else
\set until 'infinity'
\endif

WITH image(ref) AS (
    -- Not a digest: these images were built locally and never pushed, so they
    -- have no registry digest to name. Rows written after SBX-011 publishes to
    -- GHCR will carry `...@sha256:...`, which is the form 0022 documents as
    -- preferred.
    VALUES (:'image')
),
declared(name, deps_runtime) AS (
  VALUES
    ('add-data-dictionary',            'python'),
    ('add-iso3166',                    'python'),
    ('ai-written-check',               'none'),
    ('brand-guidelines',               'none'),
    ('copyright-creative-work',        'none'),
    ('course-quiz-builder',            'node'),
    ('cringe-check',                   'none'),
    ('csv-to-json',                    'python'),
    ('data-analyst',                   'python'),
    ('data-cleanliness-scan',          'python'),
    ('data-comparability',             'python'),
    ('data-shape',                     'python'),
    ('date-wrangling',                 'python'),
    ('document-format-skills',         'python'),
    ('docx',                           'python'),
    ('excel-date-to-text',             'python'),
    ('excel-deduplicate',              'python'),
    ('excel-delete',                   'python'),
    ('excel-filter',                   'python'),
    ('excel-find-duplicates',          'python'),
    ('excel-format',                   'python'),
    ('excel-freeze',                   'python'),
    ('excel-insert',                   'python'),
    ('excel-mapping-replace',          'python'),
    ('excel-merge',                    'python'),
    ('excel-regex-clean',              'python'),
    ('excel-scout',                    'python'),
    ('excel-sort',                     'python'),
    ('excel-split',                    'python'),
    ('excel-validate',                 'python'),
    ('full-review',                    'none'),
    ('handoff',                        'none'),
    ('humanizer',                      'none'),
    ('internal-comms',                 'none'),
    ('json-restructure',               'python'),
    ('line-edit',                      'none'),
    ('pdf',                            'python'),
    ('pii-flag',                       'python'),
    ('pptx',                           'python'),
    ('shorten',                        'none'),
    ('sokrati',                        'none'),
    ('standardise-country-names',      'python'),
    ('text-to-numeric',                'python'),
    ('unicode-consistency',            'python'),
    ('xlsx',                           'python')
),
-- One Run per catalog version: the newest, which is the one the report's table
-- records (a retried skill has an earlier aborted Run that the report discards).
baseline AS (
    SELECT DISTINCT ON (fork.forked_from_version_id)
           fork.forked_from_version_id AS skill_version_id,
           r.id                         AS run_id,
           COALESCE(r.finished_at, r.created_at) AS measured_at,
           EXISTS (
               SELECT 1 FROM trace_events t
               WHERE t.run_id = r.id AND t.event_type = 'skill_activation'
           ) AS activated
    FROM runs r
    JOIN skill_versions fv ON fv.id = r.skill_version_id
    JOIN skills fork       ON fork.id = fv.skill_id
    -- The baseline ran in a scratch personal workspace, never in the catalog one
    -- (iron rule 4: the catalog rows were not touched).
    JOIN workspaces bw     ON bw.id = r.workspace_id AND NOT bw.is_catalog
    WHERE fork.forked_from_version_id IS NOT NULL
      -- The window that says which image these Runs were made under. A version
      -- with no Run in it produces no row, so the other batch's measurements are
      -- left exactly as they are rather than rewritten with this image's label.
      -- Both ends are open by default, which is the original single-image case;
      -- once a second image has been measured, each batch needs its own closed
      -- interval to stay independently re-derivable.
      AND r.created_at >= :'since'::timestamptz
      AND r.created_at <  :'until'::timestamptz
    ORDER BY fork.forked_from_version_id, r.created_at DESC
)
INSERT INTO skill_runtime_compatibility
    (skill_version_id, runtime_image, capability, runtime, source_run_id, measured_at)
SELECT b.skill_version_id,
       image.ref,
       CASE WHEN b.activated THEN 'activated' ELSE 'unverified' END,
       CASE WHEN d.deps_runtime = 'python' THEN :'python_runtime' ELSE 'native' END,
       b.run_id,
       b.measured_at
FROM baseline b
JOIN skill_versions cv ON cv.id = b.skill_version_id
JOIN skills cs         ON cs.id = cv.skill_id
JOIN workspaces cw     ON cw.id = cs.workspace_id AND cw.is_catalog
JOIN declared d        ON d.name = cs.name
CROSS JOIN image
ON CONFLICT (skill_version_id, runtime_image) DO UPDATE
SET capability    = EXCLUDED.capability,
    runtime       = EXCLUDED.runtime,
    source_run_id = EXCLUDED.source_run_id,
    measured_at   = EXCLUDED.measured_at;
