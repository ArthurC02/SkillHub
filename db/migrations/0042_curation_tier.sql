-- 0042_curation_tier: record which skills have been through the PDM-002 curation
-- review, so the catalogue can say 精選 to somebody who is deciding.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- 01 §8 has described a three-tier catalogue since M0, 02:CONTENT-001 has its
-- acceptance, and PDM-002's nine-item checklist has been run: fifteen entries
-- passed it, one of them was re-judged on 2026-08-27 after a line-by-line script
-- review. None of that reached a user. `tierLabel()` returned TierIndexed
-- unconditionally, and it was right to: curation is a recorded human review and
-- nothing recorded it. This column is that record.
--
-- Two values, not three. TierExternal (外部結果) is a result that was never
-- imported — a user-submitted URL that has no row at all — so it is a state of
-- the search, not a state of this table. A CHECK listing it would be a value no
-- INSERT can ever produce.
--
-- On `skills` and not on `skill_versions`, for 0023's reason and one more.
-- 0023's: the verdict has to be revocable — CONTENT-009 exists to notice that
-- upstream content changed — and skill_versions is immutable (iron rule 4).
-- The extra one: five of PDM-002's nine checks are about *specific bytes*
-- (script line count, no likely secrets, spec valid), so a skill-level verdict
-- alone would keep the badge on bytes nobody reviewed. Hence the second column.
--
-- `curated_version_id` is the version the review actually looked at. The read
-- path shows 精選 only while it is still the newest version; a new version drops
-- the skill back to 已索引 with no operator action and no job, which is the
-- behaviour 0022 chose for the same reason (a measurement belongs to the thing
-- it measured, and a stale verdict carried forward is worse than no verdict).
--
-- ON DELETE SET NULL rather than a strict pairing CHECK. The purge path deletes
-- skill_versions and then skills in that order (governance.sql), so a NOT NULL
-- pairing would make a workspace purge fail on its own skill's own version. A
-- row left as ('curated', NULL) is therefore reachable, and the read path treats
-- it as 已索引 — fail-closed, because the reviewed bytes are the thing that can
-- no longer be produced.
ALTER TABLE skills
    ADD COLUMN curation_tier text NOT NULL DEFAULT 'indexed'
        CHECK (curation_tier IN ('curated', 'indexed')),
    ADD COLUMN curated_version_id uuid REFERENCES skill_versions (id) ON DELETE SET NULL,
    -- One direction only. An indexed row must not carry a reviewed version,
    -- because that is a verdict pretending to be absent. The other direction is
    -- deliberately unconstrained: see ON DELETE SET NULL above.
    ADD CONSTRAINT skills_indexed_has_no_curated_version
        CHECK (curation_tier <> 'indexed' OR curated_version_id IS NULL);

-- Forks do NOT carry it, and this is the opposite of what access_restriction and
-- redistribution do. Those two are facts about the *licence* of the source, so
-- they stay true in a copy. This one is a statement that a person read a
-- particular set of bytes in the catalogue; a fork is a different workspace's
-- own copy, which nobody read. Copying the badge would be exactly the
-- endorsement PDM-002 warns against, so CreateSkill does not pass these two
-- columns and every fork gets the DEFAULT.
COMMENT ON COLUMN skills.curation_tier IS
    'PDM-002 curation verdict for this skill: curated | indexed. Default indexed. Not copied onto forks. See 0042.';
COMMENT ON COLUMN skills.curated_version_id IS
    'The skill_version the curation review examined. 精選 is shown only while this is still the newest version. See 0042.';
