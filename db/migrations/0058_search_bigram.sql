-- Hybrid retrieval needs a lexical leg that can see Chinese. The english
-- tsvector (0007) tokenises nothing in a Traditional Chinese query: on the M1
-- golden query set replayed through the product SQL it scores F1@3 0.02, and
-- the hybrid query's lexical CTE contributes nothing (creation-measure/search-f1,
-- 2026-09-06). `bigram` holds the same text tokenised the way the golden set's
-- evaluator does — latin words plus CJK character bigrams — under the 'simple'
-- config, written by Go (discovery.LexicalIndexText) at index time. The owner's
-- rule: hybrid is not optional, and it is judged by F1 — with this leg admitted
-- only when it covers the whole query, F1 over golden + name + distinctive-term
-- queries is 0.88 against 0.59 for the vector leg alone.
ALTER TABLE search_documents ADD COLUMN IF NOT EXISTS bigram tsvector;
CREATE INDEX IF NOT EXISTS search_documents_bigram_idx ON search_documents USING gin (bigram);
