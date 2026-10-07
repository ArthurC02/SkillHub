DROP INDEX search_documents_embedding_idx;

DROP INDEX search_documents_catalog_order_idx;
ALTER TABLE search_documents DROP COLUMN curated;
ALTER TABLE search_documents ADD COLUMN curated boolean NOT NULL
    GENERATED ALWAYS AS (coalesce(curated_version_id = latest_version_id, false)) STORED;
CREATE INDEX search_documents_catalog_order_idx
    ON search_documents (curated DESC, verified_at DESC NULLS LAST, skill_id) WHERE listable;

ALTER TABLE skill_versions ADD CONSTRAINT skill_versions_id_skill_key UNIQUE (id, skill_id);
ALTER TABLE skill_versions ADD CONSTRAINT skill_versions_package_key
    UNIQUE (id, skill_id, package_object_key, source_path);

ALTER TABLE search_documents DROP CONSTRAINT search_documents_skill_id_fkey,
    ADD CONSTRAINT search_documents_skill_id_fkey
    FOREIGN KEY (skill_id, workspace_id) REFERENCES skills (id, workspace_id) ON DELETE CASCADE,
    ADD CONSTRAINT search_documents_latest_package_present_with_version
    CHECK ((latest_version_id IS NULL) = (latest_package_object_key IS NULL)),
    ADD CONSTRAINT search_documents_latest_version_id_fkey
    FOREIGN KEY (latest_version_id, skill_id, latest_package_object_key, latest_source_path)
    REFERENCES skill_versions (id, skill_id, package_object_key, source_path)
    ON DELETE SET NULL (latest_version_id, latest_package_object_key),
    ADD CONSTRAINT search_documents_curated_version_id_fkey
    FOREIGN KEY (curated_version_id, skill_id) REFERENCES skill_versions (id, skill_id)
    ON DELETE SET NULL (curated_version_id);
