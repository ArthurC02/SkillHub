ALTER TABLE skills
    ADD COLUMN category text
        CHECK (category IS NULL OR category IN ('documents', 'writing', 'data'));

COMMENT ON COLUMN skills.category IS
    'PDM-001 category: documents | writing | data, or NULL when the platform has not assigned one (05 R-19). Copied onto forks. See 0053.';
