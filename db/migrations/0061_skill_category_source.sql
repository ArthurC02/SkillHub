ALTER TABLE skills
    ADD COLUMN category_source text
        CHECK (category_source IS NULL OR category_source IN ('curated', 'owner')),
    ADD CONSTRAINT skills_category_and_source_together
        CHECK ((category IS NULL) = (category_source IS NULL));

UPDATE skills SET category_source = 'curated' WHERE category IS NOT NULL;

COMMENT ON COLUMN skills.category_source IS
    'Who assigned skills.category: curated (PDM-001 backfill) | owner (PUT /skills/{id}/category, 05 R-19). NULL iff category is NULL. See 0061.';
