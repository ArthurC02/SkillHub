-- name: UpsertSearchDocument :exec
INSERT INTO search_documents (skill_id, workspace_id, name, summary, bigram, updated_at)
VALUES ($1, $2, $3, $4, to_tsvector('simple', sqlc.arg(bigram_text)::text), now())
ON CONFLICT (skill_id) DO UPDATE
SET name = EXCLUDED.name, summary = EXCLUDED.summary, bigram = EXCLUDED.bigram, updated_at = now();

-- name: UpsertSearchDocumentEnriched :exec
INSERT INTO search_documents (
    skill_id, workspace_id, name, summary,
    enriched_summary, task_examples, tags, limitations, scan, embedding,
    enrichment_status, enrichment_model, enrichment_prompt_version, bigram, listable, has_script, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, to_tsvector('simple', sqlc.arg(bigram_text)::text), sqlc.arg(listable), sqlc.narg(has_script), now())
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
    listable = EXCLUDED.listable,
    has_script = EXCLUDED.has_script,
    enrichment_attempted_at = CASE
        WHEN sqlc.arg(restart_enrichment_attempts)::bool THEN NULL
        ELSE search_documents.enrichment_attempted_at END,
    updated_at = now();

-- name: ListPendingEnrichment :many
WITH candidates AS (
SELECT sd.skill_id, sd.latest_version_id AS version_id, sd.latest_package_object_key AS package_object_key,
       sd.latest_source_path AS source_path
FROM search_documents sd
WHERE sd.enrichment_status = 'pending'
  AND sd.latest_package_object_key IS NOT NULL
  AND (sd.enrichment_attempted_at IS NULL OR sd.enrichment_attempted_at < now() - @claim_lease::interval)
ORDER BY sd.enrichment_attempted_at NULLS FIRST, sd.enrichment_attempted_at, sd.updated_at, sd.skill_id
LIMIT @batch_size FOR UPDATE OF sd SKIP LOCKED
), claimed AS (
    UPDATE search_documents sd SET enrichment_attempted_at = now()
    FROM candidates c WHERE sd.skill_id = c.skill_id
    RETURNING sd.skill_id, sd.workspace_id, sd.name
)
SELECT c.skill_id, c.workspace_id, c.name, candidates.version_id, candidates.package_object_key,
       candidates.source_path
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
       s.summary, s.enriched_summary,
       s.tags, s.scan, s.verified_at,
       s.agent_capability, s.agent_runtime, s.agent_runtime_image, s.agent_measured_at,
       s.curated,
       s.category,
       s.category_source,
       count(*) OVER ()::bigint AS total_matches
FROM search_documents s
WHERE s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
  AND (s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
       OR s.bigram @@ to_tsquery('simple', nullif(sqlc.arg(bigram_query)::text, '')))
  AND s.listable
  AND (
    sqlc.narg(has_script)::bool IS NULL
    OR s.has_script = sqlc.narg(has_script)::bool
  )
  AND (
    sqlc.narg(spec_validated)::bool IS NULL
    OR (s.verified_at IS NOT NULL) = sqlc.narg(spec_validated)::bool
  )
  AND (
    sqlc.narg(agent_runtime)::text IS NULL
    OR s.agent_runtime = sqlc.narg(agent_runtime)::text
  )
  AND (
    sqlc.narg(curated)::bool IS NULL
    OR s.curated = sqlc.narg(curated)::bool
  )
  AND (
    sqlc.narg(category)::text IS NULL
    OR s.category = sqlc.narg(category)::text
  )
ORDER BY GREATEST(
    ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text)),
    ts_rank_cd(s.bigram, to_tsquery('simple', nullif(sqlc.arg(bigram_query)::text, '')))) DESC
LIMIT sqlc.arg(result_limit);

-- name: BrowseCatalogSkills :many
SELECT s.skill_id, s.name,
       s.summary, s.enriched_summary,
       s.tags, s.scan, s.verified_at,
       s.agent_capability, s.agent_runtime, s.agent_runtime_image, s.agent_measured_at,
       s.curated,
       s.category,
       s.category_source,
       count(*) OVER ()::bigint AS total_matches
FROM search_documents s
WHERE s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
  AND s.listable
  AND (
    sqlc.narg(has_script)::bool IS NULL
    OR s.has_script = sqlc.narg(has_script)::bool
  )
  AND (
    sqlc.narg(spec_validated)::bool IS NULL
    OR (s.verified_at IS NOT NULL) = sqlc.narg(spec_validated)::bool
  )
  AND (
    sqlc.narg(agent_runtime)::text IS NULL
    OR s.agent_runtime = sqlc.narg(agent_runtime)::text
  )
  AND (
    sqlc.narg(curated)::bool IS NULL
    OR s.curated = sqlc.narg(curated)::bool
  )
  AND (
    sqlc.narg(category)::text IS NULL
    OR s.category = sqlc.narg(category)::text
  )
ORDER BY s.curated DESC,
         s.verified_at DESC NULLS LAST,
         s.skill_id
LIMIT sqlc.arg(result_limit);


-- name: ListHybridSearchCandidates :many
WITH vec AS (
    SELECT s.skill_id, (s.embedding IS NULL)::bool AS unembedded,
           COALESCE(s.embedding <=> sqlc.arg(query_embedding)::vector, 0)::float8 AS distance
    FROM search_documents s
    WHERE s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
      AND s.embedding IS NOT NULL
    ORDER BY s.embedding <=> sqlc.arg(query_embedding)::vector ASC
    LIMIT sqlc.arg(vector_candidates)::int
),
fts AS (
    SELECT s.skill_id, (s.embedding IS NULL)::bool AS unembedded,
           COALESCE(s.embedding <=> sqlc.arg(query_embedding)::vector, 0)::float8 AS distance
    FROM search_documents s
    WHERE s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
      AND s.listable
      AND s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
    ORDER BY ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text)) DESC
    LIMIT sqlc.arg(fulltext_candidates)::int
),
lex AS (
    SELECT s.skill_id, (s.embedding IS NULL)::bool AS unembedded,
           COALESCE(s.embedding <=> sqlc.arg(query_embedding)::vector, 0)::float8 AS distance
    FROM search_documents s
    WHERE s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
      AND s.listable
      AND s.bigram @@ to_tsquery('simple', nullif(sqlc.arg(bigram_query)::text, ''))
    ORDER BY ts_rank_cd(s.bigram, to_tsquery('simple', nullif(sqlc.arg(bigram_query)::text, ''))) DESC
    LIMIT sqlc.arg(lexical_candidates)::int
)
SELECT skill_id, unembedded, distance, false AS lexical FROM vec
UNION ALL
SELECT skill_id, unembedded, distance, false AS lexical FROM fts
UNION ALL
SELECT skill_id, unembedded, distance, true AS lexical FROM lex;

-- name: ListHybridSearchDocuments :many
SELECT s.skill_id, s.name,
       s.summary, s.enriched_summary,
       s.tags, s.scan, s.verified_at,
       s.agent_capability, s.agent_runtime, s.agent_runtime_image, s.agent_measured_at,
       s.curated,
       s.category,
       s.category_source
FROM search_documents s
WHERE s.skill_id = ANY(sqlc.arg(skill_ids)::uuid[])
  AND s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
  AND (
    sqlc.narg(has_script)::bool IS NULL
    OR s.has_script = sqlc.narg(has_script)::bool
  )
  AND (
    sqlc.narg(spec_validated)::bool IS NULL
    OR (s.verified_at IS NOT NULL) = sqlc.narg(spec_validated)::bool
  )
  AND (
    sqlc.narg(agent_runtime)::text IS NULL
    OR s.agent_runtime = sqlc.narg(agent_runtime)::text
  )
  AND (
    sqlc.narg(curated)::bool IS NULL
    OR s.curated = sqlc.narg(curated)::bool
  )
  AND (
    sqlc.narg(category)::text IS NULL
    OR s.category = sqlc.narg(category)::text
  );

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
    latest_source_path = sqlc.arg(latest_source_path),
    curated_version_id = sqlc.narg(curated_version_id),
    curated = sqlc.arg(curated),
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

-- name: ListCatalogueDocumentsEnrichedBefore :many
SELECT sd.skill_id, (sd.embedding IS NOT NULL)::bool AS has_embedding
FROM search_documents sd
WHERE sd.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
  AND sd.enrichment_status = sqlc.arg(enriched_status)::text
  AND (sd.enrichment_prompt_version IS NULL OR sd.enrichment_prompt_version <> sqlc.arg(prompt_version)::text)
ORDER BY sd.skill_id
FOR UPDATE;

-- name: RequeueSearchDocumentEnrichment :execrows
UPDATE search_documents sd
SET enrichment_status = sqlc.arg(pending_status)::text, enrichment_attempted_at = NULL, listable = u.listable
FROM (SELECT unnest(sqlc.arg(skill_ids)::uuid[]) AS skill_id, unnest(sqlc.arg(listable)::bool[]) AS listable) u
WHERE sd.skill_id = u.skill_id;

-- name: GetCatalogReferenceFacts :one
SELECT sd.scan, sd.curated_version_id
FROM search_documents sd
WHERE sd.skill_id = sqlc.arg(skill_id)
  AND sd.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[]);

-- name: CreationLexicalSearchSkills :many
SELECT s.skill_id, s.name
FROM search_documents s
WHERE s.workspace_id = ANY(sqlc.arg(catalog_workspace_ids)::uuid[])
  AND s.listable
  AND s.bigram @@ to_tsquery('simple', sqlc.arg(query)::text)
ORDER BY ts_rank_cd(s.bigram, to_tsquery('simple', sqlc.arg(query)::text)) DESC
LIMIT sqlc.arg(result_limit)::int;

-- name: OldestPendingEnrichment :one
SELECT min(updated_at)::timestamptz FROM search_documents
WHERE enrichment_status = sqlc.arg(pending_status)::text AND latest_package_object_key IS NOT NULL;
