ALTER TABLE skill_versions
    ADD COLUMN source_path text NOT NULL DEFAULT '';

ALTER TABLE skill_versions
    ADD CONSTRAINT skill_versions_source_path_relative CHECK (
        source_path = ''
        OR (source_path !~ '^/' AND source_path !~ '/$' AND source_path !~ '(^|/)\.\.(/|$)')
    );

ALTER TABLE search_documents
    ADD COLUMN latest_source_path text NOT NULL DEFAULT '';
