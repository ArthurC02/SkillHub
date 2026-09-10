ALTER TABLE skills
    ADD COLUMN curation_tier text NOT NULL DEFAULT 'indexed'
        CHECK (curation_tier IN ('curated', 'indexed')),
    ADD COLUMN curated_version_id uuid REFERENCES skill_versions (id) ON DELETE SET NULL,
    ADD CONSTRAINT skills_indexed_has_no_curated_version
        CHECK (curation_tier <> 'indexed' OR curated_version_id IS NULL);

COMMENT ON COLUMN skills.curation_tier IS
    'PDM-002 curation verdict for this skill: curated | indexed. Default indexed. Not copied onto forks. See 0042.';
COMMENT ON COLUMN skills.curated_version_id IS
    'The skill_version the curation review examined. 精選 is shown only while this is still the newest version. See 0042.';
