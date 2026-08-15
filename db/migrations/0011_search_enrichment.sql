-- 0011_search_enrichment: index-time LLM enrichment fields on the search
-- projection (INGEST-009 增強, ADR-013 §1).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- These columns hold model-generated text and are labelled as such: ADR-013
-- requires generated content to be distinguishable from content the package
-- itself declared. `summary` keeps carrying the frontmatter description, so a
-- pending or failed enrichment never leaves the row without a human-authored
-- description to display.

ALTER TABLE search_documents
    -- Plain-language summary written by the model (ADR-013 §1). Empty until
    -- enrichment succeeds; readers fall back to `summary`.
    ADD COLUMN enriched_summary text NOT NULL DEFAULT '',
    -- Example task sentences, newline-joined, both languages. These are what
    -- makes a Chinese task description match an English SKILL.md: golden-query-set.md
    -- §3.6 traced both recall@5 misses to their absence.
    ADD COLUMN task_examples text NOT NULL DEFAULT '',
    -- Input/output/tool/dependency tags, space-joined.
    ADD COLUMN tags text NOT NULL DEFAULT '',
    -- 'enriched' only when the model text AND its embedding both landed: without
    -- a vector the document is invisible to the ranking leg, so it still needs a
    -- rebuild. Retry is a Go decision (iron rule 6) and this flag is what it
    -- reads; there is deliberately no automatic retry schedule yet.
    ADD COLUMN enrichment_status text NOT NULL DEFAULT 'pending'
        CHECK (enrichment_status IN ('pending', 'enriched')),
    -- Provenance of the generated fields (ADR-013): which model wrote them and
    -- under which prompt revision, so stale enrichments can be found and rebuilt.
    ADD COLUMN enrichment_model text,
    ADD COLUMN enrichment_prompt_version text;

-- Partial index: the backfill only ever asks for the pending rows, and once the
-- catalogue is healthy that is a small tail of a large table.
CREATE INDEX search_documents_pending_enrichment_idx
    ON search_documents (updated_at)
    WHERE enrichment_status = 'pending';
