ALTER TABLE skill_sources DROP CONSTRAINT skill_sources_source_type_check;
ALTER TABLE skill_sources
    ADD CONSTRAINT skill_sources_source_type_check
        CHECK (source_type IN ('git', 'upload', 'generated'));

ALTER TABLE skill_sources ADD COLUMN task_description text;
ALTER TABLE skill_sources ADD COLUMN generator_model text;
ALTER TABLE skill_sources ADD COLUMN generator_prompt_version text;

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

ALTER TABLE skills DROP CONSTRAINT skills_redistribution_check;
ALTER TABLE skills
    ADD CONSTRAINT skills_redistribution_check
        CHECK (redistribution IN ('allowed', 'blocked', 'unknown', 'self_supplied', 'generated'));

COMMENT ON COLUMN skills.redistribution IS
    'May a Download Artifact be produced from this skill? ''allowed'' (a verdict about the licence), ''self_supplied'' (this workspace brought the bytes) and ''generated'' (the platform wrote them for this workspace) release; ''unknown'' and ''blocked'' refuse. license_status = Confirmed must never set this on its own (CONTENT-002). Copied onto forks at fork time, like access_restriction — a fork of a generated skill stays ''generated'' (ADR-047 決策 4). See 0027, 0036 and 0037.';

