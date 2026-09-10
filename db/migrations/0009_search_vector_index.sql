CREATE INDEX IF NOT EXISTS search_documents_embedding_idx
    ON search_documents USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 1);
