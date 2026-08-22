-- 0036_self_supplied_redistribution: a fourth redistribution state, for content
-- the platform never supplied. Applied migrations are immutable: fix forward.
--
-- The hole this closes is not a bug. 0027 chose 'unknown' as the default and
-- fail-closed as the rule, and both of those are right for the population 0027
-- was looking at: a curated catalogue whose licences came from other people.
-- ADR-021 §5.3 is explicit that the misjudgement must err towards blocking.
--
-- But every skill a user brings in lands on that same default, and no product
-- path sets the column (04 乙-17). So the answer to "may I download the Skill I
-- just wrote?" was no, permanently, with a message about a licensing question
-- nobody could ever resolve. That was measured, not inferred: an MIT-licensed
-- package with its own LICENSE file, imported by an ordinary user, refused with
-- `license_unknown` (04 丙-44).
--
-- The reason it was wrong is that the gate was answering a question that was
-- not being asked. Redistribution is the platform handing A's content to B.
-- Handing a workspace back the bytes that same workspace supplied is not
-- redistribution, it is retrieval — there is no B, and the platform adds no
-- copy the user had not already made.
--
-- 'self_supplied' says exactly that and nothing wider. It is deliberately not
-- 'allowed':
--   * 'allowed' is a verdict about the CONTENT — somebody established that this
--     licence permits copies. Nobody established anything here.
--   * The day a publish-to-catalogue path exists, it must face a value that is
--     not 'allowed' and stop, rather than silently harvesting rows written
--     today. Storing 'allowed' for content nobody judged would be an error in
--     the releasing direction, which is the one direction ADR-021 §5.3 forbids.
--
-- Set at import and only for a non-catalog workspace: the seed catalogue is
-- loaded through the very same upload endpoint (tools/content/import_seed.py),
-- so a rule keyed on "was this uploaded" and not on "whose workspace" would mark
-- all 45 curated skills self-supplied and let every fork of them out. The
-- discriminator is workspaces.is_catalog, the same one PACK-001 第二層 uses.
--
-- Forks carry it verbatim, and that stays correct: Fork reads either the
-- caller's OWN workspace or a catalog workspace (skills.sql GetCatalogSkill),
-- and a catalog skill can never hold this value, so the only fork that inherits
-- it is a workspace forking its own content.
ALTER TABLE skills
    DROP CONSTRAINT IF EXISTS skills_redistribution_check;

ALTER TABLE skills
    ADD CONSTRAINT skills_redistribution_check
        CHECK (redistribution IN ('allowed', 'blocked', 'unknown', 'self_supplied'));

COMMENT ON COLUMN skills.redistribution IS
    'May a Download Artifact be produced from this skill? ''allowed'' (a verdict about the licence) and ''self_supplied'' (this workspace brought the bytes, so handing them back is not redistribution) release; ''unknown'' and ''blocked'' refuse. license_status = Confirmed must never set this on its own (CONTENT-002). Copied onto forks at fork time, like access_restriction. See 0027 and 0036.';

-- Existing rows are NOT backfilled here.
--
-- A migration cannot tell which of them a workspace supplied: the fact lives in
-- skill_sources.source_type plus workspaces.is_catalog, and reading those to
-- rewrite a gate column would make this file a policy decision executed without
-- anyone reading it. tools/content/backfill-self-supplied.sql does it as a
-- reviewable statement, run deliberately, printing what it touched.
