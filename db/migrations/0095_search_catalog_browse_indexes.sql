CREATE INDEX search_documents_catalog_order_idx
    ON search_documents (curated DESC, verified_at DESC NULLS LAST, skill_id) WHERE listable;

CREATE INDEX search_documents_listable_scope_idx
    ON search_documents (workspace_id) INCLUDE (skill_id, latest_version_id, exposure_digest) WHERE listable;
