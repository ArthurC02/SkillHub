-- name: UpsertSearchDocument :exec
INSERT INTO search_documents (skill_id, workspace_id, name, summary, bigram, updated_at)
VALUES ($1, $2, $3, $4, to_tsvector('simple', sqlc.arg(bigram_text)::text), now())
ON CONFLICT (skill_id) DO UPDATE
SET name = EXCLUDED.name, summary = EXCLUDED.summary, bigram = EXCLUDED.bigram, updated_at = now();

-- name: UpsertSearchDocumentEnriched :exec
INSERT INTO search_documents (
    skill_id, workspace_id, name, summary,
    enriched_summary, task_examples, tags, limitations, scan, embedding,
    enrichment_status, enrichment_model, enrichment_prompt_version, bigram, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, to_tsvector('simple', sqlc.arg(bigram_text)::text), now())
ON CONFLICT (skill_id) DO UPDATE
SET workspace_id = EXCLUDED.workspace_id,
    name = EXCLUDED.name,
    summary = EXCLUDED.summary,
    bigram = EXCLUDED.bigram,
    enriched_summary = EXCLUDED.enriched_summary,
    task_examples = EXCLUDED.task_examples,
    tags = EXCLUDED.tags,
    limitations = EXCLUDED.limitations,
    scan = EXCLUDED.scan,
    embedding = EXCLUDED.embedding,
    enrichment_status = EXCLUDED.enrichment_status,
    enrichment_model = EXCLUDED.enrichment_model,
    enrichment_prompt_version = EXCLUDED.enrichment_prompt_version,
    enrichment_attempted_at = CASE
        WHEN EXCLUDED.enrichment_status = 'pending' THEN NULL
        ELSE search_documents.enrichment_attempted_at END,
    updated_at = now();

-- name: ListPendingEnrichment :many
WITH candidates AS (
SELECT sd.skill_id, sd.latest_package_object_key AS package_object_key
FROM search_documents sd
WHERE sd.enrichment_status = 'pending'
  AND sd.latest_package_object_key IS NOT NULL
  AND (sd.enrichment_attempted_at IS NULL OR sd.enrichment_attempted_at < now() - interval '15 minutes')
ORDER BY sd.enrichment_attempted_at NULLS FIRST, sd.enrichment_attempted_at, sd.updated_at, sd.skill_id
LIMIT $1 FOR UPDATE OF sd SKIP LOCKED
), claimed AS (
    UPDATE search_documents sd SET enrichment_attempted_at = now()
    FROM candidates c WHERE sd.skill_id = c.skill_id
    RETURNING sd.skill_id, sd.workspace_id, sd.name
)
SELECT c.skill_id, c.workspace_id, c.name, candidates.package_object_key
FROM claimed c JOIN candidates USING (skill_id);

-- name: SearchSkills :many
SELECT s.skill_id, s.workspace_id, s.name, s.summary
FROM search_documents s
WHERE s.workspace_id = $1
  AND NOT s.generated
  AND s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
ORDER BY ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text)) DESC
LIMIT $2;

-- name: PublicSearchSkills :many
SELECT s.skill_id, s.name,
       COALESCE(NULLIF(s.enriched_summary, ''), s.summary) AS summary,
       CASE WHEN NULLIF(s.enriched_summary, '') IS NULL THEN 'package' ELSE 'model' END
           AS summary_source,
       s.tags, s.scan, s.verified_at,
       COALESCE(s.agent_capability, 'unverified') AS agent_capability,
       COALESCE(s.agent_runtime, 'unverified') AS agent_runtime,
       COALESCE(s.agent_runtime_image, '') AS agent_runtime_image,
       s.agent_measured_at,
       COALESCE(CASE WHEN s.curated_version_id = s.latest_version_id THEN 'curated' END, 'indexed')::text AS curation_tier,
       s.category,
       s.category_source,
       count(*) OVER ()::bigint AS total_matches
FROM search_documents s
WHERE s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
  AND (s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
       OR (sqlc.arg(bigram_query)::text <> ''
           AND s.bigram @@ to_tsquery('simple', sqlc.arg(bigram_query)::text)))
  AND (s.enrichment_status = 'enriched' OR s.embedding IS NOT NULL)
  AND (
    sqlc.narg(has_script)::bool IS NULL
    OR (s.scan IS NOT NULL
        AND (s.scan->'codes' @> '["script-file"]'::jsonb
             OR s.scan->'codes' @> '["embedded-script"]'::jsonb) = sqlc.narg(has_script)::bool)
  )
  AND (
    sqlc.narg(spec_validated)::bool IS NULL
    OR (s.verified_at IS NOT NULL) = sqlc.narg(spec_validated)::bool
  )
  AND (
    sqlc.narg(agent_runtime)::text IS NULL
    OR COALESCE(s.agent_runtime, 'unverified') = sqlc.narg(agent_runtime)::text
  )
  AND (
    sqlc.narg(curation_tier)::text IS NULL
    OR COALESCE(CASE WHEN s.curated_version_id = s.latest_version_id THEN 'curated' END, 'indexed') = sqlc.narg(curation_tier)::text
  )
  AND (
    sqlc.narg(category)::text IS NULL
    OR s.category = sqlc.narg(category)::text
  )
ORDER BY GREATEST(
    ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text)),
    CASE WHEN sqlc.arg(bigram_query)::text <> ''
         THEN ts_rank_cd(s.bigram, to_tsquery('simple', sqlc.arg(bigram_query)::text))
         ELSE 0 END) DESC
LIMIT sqlc.arg(result_limit);

-- name: BrowseCatalogSkills :many
SELECT s.skill_id, s.name,
       COALESCE(NULLIF(s.enriched_summary, ''), s.summary) AS summary,
       CASE WHEN NULLIF(s.enriched_summary, '') IS NULL THEN 'package' ELSE 'model' END
           AS summary_source,
       s.tags, s.scan, s.verified_at,
       COALESCE(s.agent_capability, 'unverified') AS agent_capability,
       COALESCE(s.agent_runtime, 'unverified') AS agent_runtime,
       COALESCE(s.agent_runtime_image, '') AS agent_runtime_image,
       s.agent_measured_at,
       COALESCE(CASE WHEN s.curated_version_id = s.latest_version_id THEN 'curated' END, 'indexed')::text AS curation_tier,
       s.category,
       s.category_source,
       count(*) OVER ()::bigint AS total_matches
FROM search_documents s
WHERE s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
  AND (s.enrichment_status = 'enriched' OR s.embedding IS NOT NULL)
  AND (
    sqlc.narg(has_script)::bool IS NULL
    OR (s.scan IS NOT NULL
        AND (s.scan->'codes' @> '["script-file"]'::jsonb
             OR s.scan->'codes' @> '["embedded-script"]'::jsonb) = sqlc.narg(has_script)::bool)
  )
  AND (
    sqlc.narg(spec_validated)::bool IS NULL
    OR (s.verified_at IS NOT NULL) = sqlc.narg(spec_validated)::bool
  )
  AND (
    sqlc.narg(agent_runtime)::text IS NULL
    OR COALESCE(s.agent_runtime, 'unverified') = sqlc.narg(agent_runtime)::text
  )
  AND (
    sqlc.narg(curation_tier)::text IS NULL
    OR COALESCE(CASE WHEN s.curated_version_id = s.latest_version_id THEN 'curated' END, 'indexed') = sqlc.narg(curation_tier)::text
  )
  AND (
    sqlc.narg(category)::text IS NULL
    OR s.category = sqlc.narg(category)::text
  )
ORDER BY (COALESCE(CASE WHEN s.curated_version_id = s.latest_version_id THEN 'curated' END, 'indexed') = 'curated') DESC,
         s.verified_at DESC NULLS LAST,
         s.skill_id
LIMIT sqlc.arg(result_limit);


-- name: PublicHybridSearchSkills :many
WITH vec AS (
    SELECT s.skill_id, s.embedding <=> sqlc.arg(query_embedding)::vector AS distance
    FROM search_documents s
    WHERE s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
      AND s.embedding IS NOT NULL
    ORDER BY s.embedding <=> sqlc.arg(query_embedding)::vector ASC
    LIMIT 50
),
fts AS (
    SELECT s.skill_id, s.embedding <=> sqlc.arg(query_embedding)::vector AS distance
    FROM search_documents s
    WHERE s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
      AND (s.enrichment_status = 'enriched' OR s.embedding IS NOT NULL)
      AND s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
    ORDER BY ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text)) DESC
    LIMIT 50
),
lex AS (
    SELECT s.skill_id, s.embedding <=> sqlc.arg(query_embedding)::vector AS distance
    FROM search_documents s
    WHERE s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
      AND (s.enrichment_status = 'enriched' OR s.embedding IS NOT NULL)
      AND sqlc.arg(bigram_query)::text <> ''
      AND s.bigram @@ to_tsquery('simple', sqlc.arg(bigram_query)::text)
    ORDER BY ts_rank_cd(s.bigram, to_tsquery('simple', sqlc.arg(bigram_query)::text)) DESC
    LIMIT 5
),
candidates AS (
    SELECT skill_id, min(distance) AS distance, bool_or(covered) AS covered
    FROM (
        SELECT skill_id, distance, false AS covered FROM vec
        UNION ALL
        SELECT skill_id, distance, false AS covered FROM fts
        UNION ALL
        SELECT skill_id, distance, true AS covered FROM lex
    ) legs
    GROUP BY skill_id
)
SELECT c.skill_id, s.name,
       COALESCE(NULLIF(s.enriched_summary, ''), s.summary) AS summary,
       CASE WHEN NULLIF(s.enriched_summary, '') IS NULL THEN 'package' ELSE 'model' END
           AS summary_source,
       s.tags, s.scan, s.verified_at,
       COALESCE(s.agent_capability, 'unverified') AS agent_capability,
       COALESCE(s.agent_runtime, 'unverified') AS agent_runtime,
       COALESCE(s.agent_runtime_image, '') AS agent_runtime_image,
       s.agent_measured_at,
       COALESCE(CASE WHEN s.curated_version_id = s.latest_version_id THEN 'curated' END, 'indexed')::text AS curation_tier,
       s.category,
       s.category_source,
       (1 - COALESCE(c.distance, 1))::float8 AS rank,
       (c.distance IS NULL)::bool AS unranked,
       c.covered AS lexical_covered,
       count(*) OVER ()::bigint AS total_matches
FROM candidates c
JOIN search_documents s ON s.skill_id = c.skill_id
WHERE (c.covered OR c.distance IS NULL OR c.distance <= sqlc.arg(max_distance)::float8)
  AND (
    sqlc.narg(has_script)::bool IS NULL
    OR (s.scan IS NOT NULL
        AND (s.scan->'codes' @> '["script-file"]'::jsonb
             OR s.scan->'codes' @> '["embedded-script"]'::jsonb) = sqlc.narg(has_script)::bool)
  )
  AND (
    sqlc.narg(spec_validated)::bool IS NULL
    OR (s.verified_at IS NOT NULL) = sqlc.narg(spec_validated)::bool
  )
  AND (
    sqlc.narg(agent_runtime)::text IS NULL
    OR COALESCE(s.agent_runtime, 'unverified') = sqlc.narg(agent_runtime)::text
  )
  AND (
    sqlc.narg(curation_tier)::text IS NULL
    OR COALESCE(CASE WHEN s.curated_version_id = s.latest_version_id THEN 'curated' END, 'indexed') = sqlc.narg(curation_tier)::text
  )
  AND (
    sqlc.narg(category)::text IS NULL
    OR s.category = sqlc.narg(category)::text
  )
ORDER BY (lower(s.name) = lower(btrim(sqlc.arg(query)::text))) DESC,
         c.covered DESC,
         c.distance ASC NULLS LAST
LIMIT sqlc.arg(result_limit);

-- name: ReindexAll :execrows
INSERT INTO search_documents (skill_id, workspace_id, name, summary, generated, updated_at)
SELECT unnest(sqlc.arg(skill_ids)::uuid[]), unnest(sqlc.arg(workspace_ids)::uuid[]),
       unnest(sqlc.arg(names)::text[]), unnest(sqlc.arg(summaries)::text[]),
       unnest(sqlc.arg(generated)::bool[]), now()
ON CONFLICT (skill_id) DO UPDATE
SET workspace_id = EXCLUDED.workspace_id, name = EXCLUDED.name,
    summary = EXCLUDED.summary;

-- name: DeleteSearchDocument :exec
DELETE FROM search_documents WHERE skill_id = $1 AND workspace_id = $2;

-- name: ListSearchDocumentSkillIDs :many
SELECT skill_id FROM search_documents ORDER BY skill_id;

-- name: PruneDeletedSearchDocuments :execrows
DELETE FROM search_documents WHERE skill_id = ANY(sqlc.arg(skill_ids)::uuid[]);

-- name: SetSearchDocumentListing :exec
UPDATE search_documents
SET generated = sqlc.arg(generated),
    category = sqlc.narg(category),
    category_source = sqlc.narg(category_source),
    latest_version_id = sqlc.narg(latest_version_id),
    verified_at = sqlc.narg(verified_at),
    latest_package_object_key = sqlc.narg(latest_package_object_key),
    curated_version_id = sqlc.narg(curated_version_id),
    agent_capability = sqlc.narg(agent_capability),
    agent_runtime = sqlc.narg(agent_runtime),
    agent_runtime_image = sqlc.narg(agent_runtime_image),
    agent_measured_at = sqlc.narg(agent_measured_at)
WHERE skill_id = sqlc.arg(skill_id);

-- name: ListSkillScans :many
SELECT skill_id, scan
FROM search_documents
WHERE workspace_id = $1 AND skill_id = ANY(sqlc.arg(skill_ids)::uuid[]);

-- name: ListCatalogSkillScans :many
SELECT sd.skill_id, sd.scan
FROM search_documents sd
WHERE sd.skill_id = ANY(sqlc.arg(skill_ids)::uuid[])
  AND sd.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[]);

-- name: ListSearchDocumentsMissingBigram :many
SELECT skill_id, name, summary, enriched_summary, task_examples, tags
FROM search_documents
WHERE bigram IS NULL
ORDER BY skill_id
LIMIT sqlc.arg(result_limit)::int;

-- name: SetSearchDocumentBigram :exec
UPDATE search_documents
SET bigram = to_tsvector('simple', sqlc.arg(bigram_text)::text)
WHERE skill_id = $1;

-- name: ResetCatalogueEnrichmentBefore :execrows
UPDATE search_documents sd
SET enrichment_status = 'pending', enrichment_attempted_at = NULL
WHERE sd.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
  AND sd.enrichment_status = 'enriched'
  AND COALESCE(sd.enrichment_prompt_version, '') <> sqlc.arg(prompt_version)::text;

-- name: GetCatalogReferenceFacts :one
SELECT sd.scan,
       COALESCE(sd.curated_version_id = sqlc.arg(version_id)::uuid, false)::bool AS curated
FROM search_documents sd
WHERE sd.skill_id = sqlc.arg(skill_id)
  AND sd.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[]);

-- name: CreationLexicalSearchSkills :many
SELECT s.skill_id, s.name
FROM search_documents s
WHERE s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
  AND (s.enrichment_status = 'enriched' OR s.embedding IS NOT NULL)
  AND s.bigram @@ to_tsquery('simple', sqlc.arg(query)::text)
ORDER BY ts_rank_cd(s.bigram, to_tsquery('simple', sqlc.arg(query)::text)) DESC
LIMIT sqlc.arg(result_limit)::int;
