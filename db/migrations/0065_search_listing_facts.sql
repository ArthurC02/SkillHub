ALTER TABLE search_documents
    ADD COLUMN generated boolean NOT NULL DEFAULT false,
    ADD COLUMN category text,
    ADD COLUMN category_source text,
    ADD COLUMN latest_version_id uuid,
    ADD COLUMN verified_at timestamptz,
    ADD COLUMN latest_package_object_key text,
    ADD COLUMN curated_version_id uuid,
    ADD COLUMN agent_capability text,
    ADD COLUMN agent_runtime text,
    ADD COLUMN agent_runtime_image text,
    ADD COLUMN agent_measured_at timestamptz;

DELETE FROM search_documents sd
WHERE NOT EXISTS (
    SELECT 1 FROM skills sk
    WHERE sk.id = sd.skill_id AND sk.deleted_at IS NULL AND sk.takedown_at IS NULL
);

UPDATE search_documents sd
SET generated = f.redistribution = 'generated',
    category = f.category,
    category_source = f.category_source,
    latest_version_id = f.version_id,
    verified_at = f.version_created_at,
    latest_package_object_key = f.package_object_key,
    curated_version_id = f.curated_version_id,
    agent_capability = f.capability,
    agent_runtime = f.runtime,
    agent_runtime_image = f.runtime_image,
    agent_measured_at = f.measured_at
FROM (
    SELECT sk.id, sk.redistribution, sk.category, sk.category_source,
           CASE WHEN sk.curation_tier = 'curated' THEN sk.curated_version_id END AS curated_version_id,
           ver.id AS version_id, ver.created_at AS version_created_at, ver.package_object_key,
           cmp.capability, cmp.runtime, cmp.runtime_image, cmp.measured_at
    FROM skills sk
LEFT JOIN LATERAL (
    SELECT v.id, v.created_at, v.package_object_key
    FROM skill_versions v
    WHERE v.skill_id = sk.id
    ORDER BY v.version_number DESC
    LIMIT 1
) ver ON true
LEFT JOIN LATERAL (
    SELECT c.capability, c.runtime, c.runtime_image, c.measured_at
    FROM skill_runtime_compatibility c
    WHERE c.skill_version_id = ver.id
    ORDER BY c.measured_at DESC
    LIMIT 1
) cmp ON true
) f
WHERE f.id = sd.skill_id;
