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
-- DISTINCT ON keeps one row per version; paired with ORDER BY ... created_at
-- DESC below, that row is the newest Run.
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
    JOIN workspaces bw     ON bw.id = r.workspace_id AND NOT bw.is_catalog
    WHERE fork.forked_from_version_id IS NOT NULL
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
