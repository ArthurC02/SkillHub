-- 0037_generated_skill_provenance: a package the platform wrote is neither a
-- fetch nor an upload, and handing it back is neither redistribution nor
-- self-supply (GEN-005, ADR-046, ADR-047 決策 4).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

-- ── skill_sources.source_type gains 'generated' ──────────────────────────────
--
-- 03:GEN-005 named only skills.redistribution. It is not enough: 0003 constrains
-- source_type to ('git', 'upload'), so a generated package cannot record where
-- it came from at all, and reusing 'upload' would be a lie with two
-- consequences rather than one. `02:GEN-002` requires the source to read "由平台
-- 依你的任務描述生成" and explicitly forbids showing it as unknown; and
-- admission's redistributionFor() keys on the workspace, so an upload-shaped
-- row would silently take self_supplied — the exact conflation ADR-047 決策 4
-- rules against.
ALTER TABLE skill_sources DROP CONSTRAINT skill_sources_source_type_check;
ALTER TABLE skill_sources
    ADD CONSTRAINT skill_sources_source_type_check
        CHECK (source_type IN ('git', 'upload', 'generated'));

-- What the model was asked and what answered, so the row reproduces the package
-- sitting in the workspace (02:GEN-002, ADR-047 決策 1 — the platform does not
-- edit the bytes, which is only meaningful if the inputs are recorded).
--
-- Nullable and unconstrained for the other two source types: an upload has no
-- task description, and a NOT NULL here would make every existing row a lie
-- with an empty string in it.
ALTER TABLE skill_sources ADD COLUMN task_description text;
ALTER TABLE skill_sources ADD COLUMN generator_model text;
ALTER TABLE skill_sources ADD COLUMN generator_prompt_version text;

-- Generated rows must carry all three. The check is one-directional on purpose:
-- it says nothing about git and upload rows, which have none of them.
ALTER TABLE skill_sources
    ADD CONSTRAINT skill_sources_generated_needs_provenance
        CHECK (
            source_type <> 'generated'
            OR (
                task_description IS NOT NULL
                AND generator_model IS NOT NULL
                AND generator_prompt_version IS NOT NULL
            )
        );

COMMENT ON COLUMN skill_sources.task_description IS
    'The user''s own words that produced a generated package (GEN-001). NULL for git and upload. Free text the user submitted, so it is subject to NFR-002 deletion like any other user content — unlike the analytics events, which never record query text at all (02:O11Y-004).';
COMMENT ON COLUMN skill_sources.generator_model IS
    'Model id that wrote a generated package, as apps/llm reported it. NULL for git and upload.';
COMMENT ON COLUMN skill_sources.generator_prompt_version IS
    'Generator prompt revision, e.g. generate-skill/v1. NULL for git and upload. Together with task_description and generator_model this is what lets someone re-derive the package (ADR-047 決策 1).';

-- ── skills.redistribution gains 'generated' ──────────────────────────────────
--
-- A fifth value rather than reusing self_supplied. Both release a download and
-- both refuse to be `allowed`, so the difference looks cosmetic until the
-- question ADR-045 left open is asked: may this be published to the catalogue?
-- For self_supplied that asks whether the user had the right to redistribute
-- someone else's bytes. For generated it asks who owns what a model wrote.
-- Two different questions sharing one value means one of them eventually gets
-- released by an answer to the other (ADR-047 決策 4).
--
-- Deliberately NOT 'allowed', same as self_supplied: every future publishing
-- path has to stop and ask for a verdict.
ALTER TABLE skills DROP CONSTRAINT skills_redistribution_check;
ALTER TABLE skills
    ADD CONSTRAINT skills_redistribution_check
        CHECK (redistribution IN ('allowed', 'blocked', 'unknown', 'self_supplied', 'generated'));

COMMENT ON COLUMN skills.redistribution IS
    'May a Download Artifact be produced from this skill? ''allowed'' (a verdict about the licence), ''self_supplied'' (this workspace brought the bytes) and ''generated'' (the platform wrote them for this workspace) release; ''unknown'' and ''blocked'' refuse. license_status = Confirmed must never set this on its own (CONTENT-002). Copied onto forks at fork time, like access_restriction — a fork of a generated skill stays ''generated'' (ADR-047 決策 4). See 0027, 0036 and 0037.';

-- No backfill. Not one existing row was generated: the path that writes them
-- does not exist yet, which is the whole reason this file is here.
