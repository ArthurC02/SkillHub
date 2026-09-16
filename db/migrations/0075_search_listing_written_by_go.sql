ALTER TABLE search_documents
    ADD COLUMN curated boolean NOT NULL DEFAULT false,
    ADD COLUMN listable boolean NOT NULL DEFAULT false;

UPDATE search_documents
SET curated = COALESCE(curated_version_id = latest_version_id, false),
    listable = enrichment_status = 'enriched' OR embedding IS NOT NULL,
    agent_capability = COALESCE(agent_capability, 'unverified'),
    agent_runtime = COALESCE(agent_runtime, 'unverified'),
    agent_runtime_image = COALESCE(agent_runtime_image, '');
