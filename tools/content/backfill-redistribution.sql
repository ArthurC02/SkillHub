-- Backfill: may a Download Artifact be produced from this skill (0027,
-- skills.redistribution).
--
-- Decision and criteria: ADR-027 decision 4. Three values, NOT NULL, default
-- 'unknown', and only 'allowed' releases — 'unknown' blocks. This is the second
-- of the two locks: skills.access_restriction (0023) is a hold somebody pressed
-- by hand, this one is a property of the content that every skill needs an
-- answer to. Neither substitutes for the other.
--
-- Data, not schema, so it is not in the migration — same reasoning as the other
-- two files in this directory: a fresh database has no skills, and a migration
-- that wrote a licensing verdict about rows it cannot see would be a fixture
-- pretending to be a decision. Re-runnable: the verdict is derived, and the
-- UPDATE only touches rows whose current value differs.
--
-- WHAT IT DERIVES FROM, AND WHY NOT FROM seed-skills.json
--
-- The verdict is computed from skill_versions.license_expression, which is what
-- ADR-027's criteria are written against and what a user-uploaded skill will
-- also have. tools/content/seed-skills.json's curation fields
-- (sources.<id>.license / .redistributable / .source_available) were the input
-- to a hand cross-check, not to this script: transcribing 45 rows into a VALUES
-- list is exactly how the `deps` field got under-recorded once already
-- (seed-skills.json, skills[].deps). The cross-check result for the 45 seeds is
-- recorded in docs/plans/mvp/m4/report-profiles-and-redistribution.md; the only
-- literal data below is the licence classification itself.
--
-- THE CRITERIA (ADR-027 decision 4, in order)
--
--   1. license_expression IS NULL          -> unknown -> blocked (DISC-003: an
--      unknown licence must not imply free redistribution)
--   2. a known source-available term       -> blocked
--   3. on the OSI redistribution list      -> allowed
--   4. recognised but unclassifiable       -> unknown -> blocked
--   5. license_status = Confirmed is NEVER a release condition on its own
--      (02:CONTENT-002 — confirmed means verified, not redistributable). It is
--      absent from this file for that reason, not by oversight.
--
-- Criterion 2 is evaluated before 3 so that any future term carrying both a
-- source-available marker and an OSI-looking string lands on the blocking side.
--
-- COPYLEFT IS DELIBERATELY ABSENT FROM THE ALLOW LIST
--
-- GPL / LGPL / AGPL permit redistribution, so a purist reading would allow
-- them; they also attach obligations to the act of distributing (source offer,
-- licence text propagation) that the packager does not implement today. They
-- therefore fall through to 'unknown' and are blocked. That is a conservative
-- misfire in the safe direction, it costs nothing right now (no seed skill is
-- copyleft), and the upgrade path is one row in the list below plus whatever
-- INSTALL.md has to carry.
--
-- IDENTIFICATION AND FORK PROPAGATION
--
-- Roots are every live skill that is not a fork; forks inherit their root's
-- verdict, recursively, because ADR-027 has redistribution copied onto forks at
-- fork time and the M2 baseline forked all 45 catalogue skills before the column
-- existed. Same shape as restrict-anthropic-sa-display.sql, and the same reason.
-- A live skill that is neither reached leaves the 'unknown' default and is
-- therefore blocked, which is the intended direction for anything nobody has
-- classified.
--
-- Precondition: migration 0027 must be applied first — it is the file that adds
-- skills.redistribution. Until then this script fails on the column, which is
-- the correct failure: there is nothing to back-fill into.
--
-- Run with:
--   psql -v ON_ERROR_STOP=1 --single-transaction -f tools/content/backfill-redistribution.sql
--
-- It ends with two SELECTs: the three-way tally, and the list of everything left
-- 'unknown' — the rows a human still has to decide.

WITH RECURSIVE
-- SPDX ids whose terms permit redistribution of an unmodified or modified copy.
-- Short on purpose: anything not named here is 'unknown' and blocked, so the
-- cost of omission is a false block, not a leak.
osi_redistributable(expression) AS (
    VALUES ('0BSD'),
           ('Apache-2.0'),
           ('BSD-2-Clause'),
           ('BSD-3-Clause'),
           ('CC0-1.0'),
           ('ISC'),
           ('MIT'),
           ('MPL-2.0'),
           ('Unlicense')
),
-- Known source-available (non-OSI) terms. `LicenseRef-Anthropic-Source-Available`
-- is the spelling seed-skills.json uses; `Proprietary%` is what the four
-- anthropics/skills packages actually declare in their SKILL.md frontmatter, and
-- it is that declared string, not the curation note, that reaches the database.
source_available(pattern) AS (
    VALUES ('LicenseRef-%Source-Available%'),
           ('Proprietary%')
),
-- The licence of record is the newest version's: it is the one a package would
-- be built from, and the pairing CHECK in 0012 keeps expression and source
-- together on every row.
latest AS (
    SELECT DISTINCT ON (sv.skill_id)
           sv.skill_id,
           btrim(COALESCE(sv.license_expression, '')) AS expression
    FROM skill_versions sv
    ORDER BY sv.skill_id, sv.version_number DESC
),
roots AS (
    SELECT sk.id,
           CASE
               -- NULL and the two strings ADR-021 decision 7 forbids storing are
               -- the same fact: nobody knows.
               WHEN l.expression IS NULL OR l.expression IN ('', 'NOASSERTION', 'NONE')
                   THEN 'unknown'
               WHEN EXISTS (
                   SELECT 1 FROM source_available sa WHERE l.expression ILIKE sa.pattern
               ) THEN 'blocked'
               WHEN EXISTS (
                   SELECT 1 FROM osi_redistributable o WHERE o.expression = l.expression
               ) THEN 'allowed'
               ELSE 'unknown'
           END AS verdict
    FROM skills sk
    LEFT JOIN latest l ON l.skill_id = sk.id
    WHERE sk.deleted_at IS NULL
      AND sk.forked_from_skill_id IS NULL
),
lineage(id, verdict) AS (
    SELECT id, verdict FROM roots
    UNION
    SELECT sk.id, lg.verdict
    FROM skills sk
    JOIN lineage lg ON sk.forked_from_skill_id = lg.id
    WHERE sk.deleted_at IS NULL
)
UPDATE skills sk
SET redistribution = lg.verdict,
    updated_at     = now()
FROM lineage lg
WHERE sk.id = lg.id
  AND sk.redistribution IS DISTINCT FROM lg.verdict;

-- The tally. Catalogue and forks are separated because they are the same
-- verdicts counted twice, and a single total hides that.
SELECT w.is_catalog,
       sk.redistribution,
       count(*)
FROM skills sk
JOIN workspaces w ON w.id = sk.workspace_id
WHERE sk.deleted_at IS NULL
GROUP BY 1, 2
ORDER BY 1 DESC, 2;

-- Everything still blocked for want of a decision. An empty result is the goal;
-- a non-empty one is the work list, not a failure of this script.
SELECT sk.name,
       w.is_catalog,
       COALESCE(lv.license_expression, '(none)') AS license_expression,
       COALESCE(lv.license_source, '(none)')     AS license_source
FROM skills sk
JOIN workspaces w ON w.id = sk.workspace_id
LEFT JOIN LATERAL (
    SELECT sv.license_expression, sv.license_source
    FROM skill_versions sv
    WHERE sv.skill_id = sk.id
    ORDER BY sv.version_number DESC
    LIMIT 1
) lv ON true
WHERE sk.deleted_at IS NULL
  AND sk.redistribution = 'unknown'
ORDER BY w.is_catalog DESC, sk.name;
