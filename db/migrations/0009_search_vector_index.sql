-- 0009_search_vector_index: add IVFFlat index on search_documents.embedding for
-- vector similarity search (ADR-013 hybrid retrieval). The embedding column was
-- added in 0007; this migration adds the index now that the embedding pipeline
-- is landing.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

-- IVFFlat with cosine distance. lists=1 is appropriate for small corpus (< 1000);
-- increase when corpus grows past ~10k documents.
CREATE INDEX IF NOT EXISTS search_documents_embedding_idx
    ON search_documents USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 1);
