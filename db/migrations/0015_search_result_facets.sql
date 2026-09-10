ALTER TABLE search_documents
    DROP COLUMN tags,
    ADD COLUMN tags jsonb,
    ADD COLUMN limitations text NOT NULL DEFAULT '',
    ADD COLUMN scan jsonb;
