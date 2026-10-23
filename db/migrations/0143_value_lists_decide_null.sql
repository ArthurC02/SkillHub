ALTER TABLE evaluations
    DROP CONSTRAINT evaluations_suggestion_outcome_known,
    ADD CONSTRAINT evaluations_suggestion_outcome_known
    CHECK (suggestion_outcome IS NULL OR suggestion_outcome IN (
        'restricted', 'not_needed', 'no_evidence', 'unavailable', 'none_proposed', 'none_kept', 'kept')) NOT VALID;

ALTER TABLE publication_releases
    DROP CONSTRAINT publication_releases_declared_license_check,
    ADD CONSTRAINT publication_releases_declared_license_check
    CHECK (declared_license IS NULL OR declared_license IN (
        'Apache-2.0', 'MIT', 'ISC', 'BSD-2-Clause', 'BSD-3-Clause', 'GPL-2.0', 'GPL-3.0', 'LGPL-3.0', 'AGPL-3.0',
        'MPL-2.0', 'Unlicense', 'CC0-1.0', 'CC-BY-4.0', 'CC-BY-SA-4.0')) NOT VALID;

ALTER TABLE search_documents
    ADD CONSTRAINT search_documents_category_check
    CHECK (category IS NULL OR category IN ('documents', 'writing', 'data')) NOT VALID;

COMMENT ON COLUMN search_documents.category IS
    'The catalog category the skill is filed under, copied from skills.category: documents, writing or data. NULL when it has none.';

ALTER TABLE model_call_budgets
    ADD CONSTRAINT model_call_budgets_kind_check CHECK (btrim(kind) <> '') NOT VALID;
