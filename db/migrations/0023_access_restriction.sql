-- 0023_access_restriction: hold a skill's materials back while a licensing
-- question about them is open, without removing the skill from the catalogue.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- Why a new column rather than the takedown columns that already exist: a
-- takedown withdraws the skill (410 Gone, dropped from the index, not forkable).
-- This is the opposite shape — the skill stays listed, stays searchable, keeps
-- its summary, and only the paths that hand the package's own bytes to somebody
-- are closed. Reusing takedown_at would delete the thing the decision was
-- explicitly designed to keep (m2/anthropic-sa-license-memo.md 方案 C).
--
-- Named `access_restriction` and not `display_restricted`, because it is not
-- only about display: it also refuses new Runs. What it gates, exactly —
--
--   closed:  GET /api/skills/{id}/files  (SKILL.md full text and the file tree)
--            POST /skills/{id}/runs      (422, no sandbox copy is made)
--   open:    search and its ranking, the model-written summary, limitations,
--            license and provenance, the risk block, the source link
--
-- The open half is deliberate: those are the platform's own statements about the
-- skill, not reproductions of it, and the decision under review is precisely
-- whether reproduction is permitted. Download packaging is unaffected here
-- because it is already blocked for this content by CONTENT-004/ADR-012 — a
-- second lock on the same door would just be a second thing to forget.
--
-- On `skills` and not on `skill_versions`: a licence covers the source, so it
-- covers every version of that skill including ones not imported yet, and the
-- restriction has to be liftable — skill_versions is immutable (iron rule 4), so
-- a mutable hold cannot live there.
--
-- NULL = no restriction, which is what every existing row gets. A non-null value
-- is the reason code, currently only 'license-review'. Not an enum: the set is
-- expected to change with the reasons, and a CHECK listing one value would be a
-- migration every time somebody has a new reason to hold content back. The empty
-- string is refused, because "restricted, reason unrecorded" is the state that
-- makes a hold impossible to review or lift.
ALTER TABLE skills
    ADD COLUMN access_restriction text
        CHECK (access_restriction IS NULL OR btrim(access_restriction) <> '');

-- Forks carry it. A fork is another copy of the same materials under the same
-- licence, so the hold has to travel with it exactly as the licence expression
-- and its provenance tier already do (WS-001, ADR-021); the alternative is a
-- lineage walk on every read, which a fork of a fork would then escape.
COMMENT ON COLUMN skills.access_restriction IS
    'Reason code for a licensing hold on the package materials; NULL = none. Set by review, copied onto forks at fork time. See 0023.';
