ALTER TABLE search_documents ADD COLUMN IF NOT EXISTS bigram tsvector;
CREATE INDEX IF NOT EXISTS search_documents_bigram_idx ON search_documents USING gin (bigram);
