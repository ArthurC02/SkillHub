ALTER TABLE search_documents ADD COLUMN enrichment_findings jsonb;

COMMENT ON COLUMN search_documents.enrichment_findings IS
    'Where the enrichment disagrees with the skill''s own document, as a JSON array of {rule, field, token}: a runtime the document needs but the limitations never state, an appraisal such as "professional" the document never made, or non-English text in an English task example. The tokens come from the platform''s own vocabulary, never from the enrichment. An empty array means the enrichment was checked and nothing disagreed; NULL means it was never checked, either because it is still pending or because it was enriched before the check existed.';
