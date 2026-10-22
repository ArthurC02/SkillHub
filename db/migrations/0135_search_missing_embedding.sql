CREATE INDEX search_documents_missing_embedding_idx
    ON search_documents (enrichment_attempted_at NULLS FIRST, updated_at, skill_id)
    WHERE enrichment_status = 'enriched' AND embedding IS NULL;
