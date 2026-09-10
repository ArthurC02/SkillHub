ALTER TABLE search_documents
    ADD COLUMN enriched_summary text NOT NULL DEFAULT '',
    ADD COLUMN task_examples text NOT NULL DEFAULT '',
    ADD COLUMN tags text NOT NULL DEFAULT '',
    ADD COLUMN enrichment_status text NOT NULL DEFAULT 'pending'
        CHECK (enrichment_status IN ('pending', 'enriched')),
    ADD COLUMN enrichment_model text,
    ADD COLUMN enrichment_prompt_version text;

CREATE INDEX search_documents_pending_enrichment_idx
    ON search_documents (updated_at)
    WHERE enrichment_status = 'pending';
