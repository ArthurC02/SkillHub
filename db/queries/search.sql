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
SELECT sd.skill_id, sv.package_object_key
FROM search_documents sd
JOIN skills sk ON sk.id = sd.skill_id AND sk.deleted_at IS NULL AND sk.takedown_at IS NULL
JOIN LATERAL (
    SELECT v.package_object_key
    FROM skill_versions v
    WHERE v.skill_id = sd.skill_id
    ORDER BY v.version_number DESC
    LIMIT 1
) sv ON true
WHERE sd.enrichment_status = 'pending'
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
JOIN skills sk ON sk.id = s.skill_id AND sk.redistribution <> 'generated'
WHERE s.workspace_id = $1
  AND s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
ORDER BY ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text)) DESC
LIMIT $2;

-- name: PublicSearchSkills :many
SELECT s.skill_id, s.name,
       COALESCE(NULLIF(s.enriched_summary, ''), s.summary) AS summary,
       CASE WHEN NULLIF(s.enriched_summary, '') IS NULL THEN 'package' ELSE 'model' END
           AS summary_source,
       s.tags, s.scan, ver.created_at AS verified_at,
       COALESCE(cmp.capability, 'unverified') AS agent_capability,
       COALESCE(cmp.runtime, 'unverified') AS agent_runtime,
       COALESCE(cmp.runtime_image, '') AS agent_runtime_image,
       cmp.measured_at AS agent_measured_at,
       COALESCE(cur.tier, 'indexed') AS curation_tier,
       cur.category,
       cur.category_source,
       count(*) OVER ()::bigint AS total_matches
FROM search_documents s
JOIN workspaces w ON w.id = s.workspace_id AND w.is_catalog
LEFT JOIN LATERAL (
    SELECT v.id, v.created_at
    FROM skill_versions v
    WHERE v.skill_id = s.skill_id
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
LEFT JOIN LATERAL (
    SELECT CASE
        WHEN sk.curation_tier = 'curated' AND sk.curated_version_id = ver.id
        THEN 'curated' ELSE 'indexed'
    END AS tier,
    sk.category, sk.category_source
    FROM skills sk
    WHERE sk.id = s.skill_id
) cur ON true
WHERE (s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
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
    OR (ver.created_at IS NOT NULL) = sqlc.narg(spec_validated)::bool
  )
  AND (
    sqlc.narg(agent_runtime)::text IS NULL
    OR COALESCE(cmp.runtime, 'unverified') = sqlc.narg(agent_runtime)::text
  )
  AND (
    sqlc.narg(curation_tier)::text IS NULL
    OR COALESCE(cur.tier, 'indexed') = sqlc.narg(curation_tier)::text
  )
  AND (
    sqlc.narg(category)::text IS NULL
    OR cur.category = sqlc.narg(category)::text
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
       s.tags, s.scan, ver.created_at AS verified_at,
       COALESCE(cmp.capability, 'unverified') AS agent_capability,
       COALESCE(cmp.runtime, 'unverified') AS agent_runtime,
       COALESCE(cmp.runtime_image, '') AS agent_runtime_image,
       cmp.measured_at AS agent_measured_at,
       COALESCE(cur.tier, 'indexed') AS curation_tier,
       cur.category,
       cur.category_source,
       count(*) OVER ()::bigint AS total_matches
FROM search_documents s
JOIN workspaces w ON w.id = s.workspace_id AND w.is_catalog
LEFT JOIN LATERAL (
    SELECT v.id, v.created_at
    FROM skill_versions v
    WHERE v.skill_id = s.skill_id
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
LEFT JOIN LATERAL (
    SELECT CASE
        WHEN sk.curation_tier = 'curated' AND sk.curated_version_id = ver.id
        THEN 'curated' ELSE 'indexed'
    END AS tier,
    sk.category, sk.category_source
    FROM skills sk
    WHERE sk.id = s.skill_id
) cur ON true
WHERE (s.enrichment_status = 'enriched' OR s.embedding IS NOT NULL)
  AND (
    sqlc.narg(has_script)::bool IS NULL
    OR (s.scan IS NOT NULL
        AND (s.scan->'codes' @> '["script-file"]'::jsonb
             OR s.scan->'codes' @> '["embedded-script"]'::jsonb) = sqlc.narg(has_script)::bool)
  )
  AND (
    sqlc.narg(spec_validated)::bool IS NULL
    OR (ver.created_at IS NOT NULL) = sqlc.narg(spec_validated)::bool
  )
  AND (
    sqlc.narg(agent_runtime)::text IS NULL
    OR COALESCE(cmp.runtime, 'unverified') = sqlc.narg(agent_runtime)::text
  )
  AND (
    sqlc.narg(curation_tier)::text IS NULL
    OR COALESCE(cur.tier, 'indexed') = sqlc.narg(curation_tier)::text
  )
  AND (
    sqlc.narg(category)::text IS NULL
    OR cur.category = sqlc.narg(category)::text
  )
ORDER BY (COALESCE(cur.tier, 'indexed') = 'curated') DESC,
         ver.created_at DESC NULLS LAST,
         s.skill_id
LIMIT sqlc.arg(result_limit);


-- name: PublicHybridSearchSkills :many
WITH vec AS (
    SELECT s.skill_id, s.embedding <=> sqlc.arg(query_embedding)::vector AS distance
    FROM search_documents s
    JOIN workspaces w ON w.id = s.workspace_id AND w.is_catalog
    WHERE s.embedding IS NOT NULL
    ORDER BY s.embedding <=> sqlc.arg(query_embedding)::vector ASC
    LIMIT 50
),
fts AS (
    SELECT s.skill_id, s.embedding <=> sqlc.arg(query_embedding)::vector AS distance
    FROM search_documents s
    JOIN workspaces w ON w.id = s.workspace_id AND w.is_catalog
    WHERE (s.enrichment_status = 'enriched' OR s.embedding IS NOT NULL)
      AND s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
    ORDER BY ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text)) DESC
    LIMIT 50
),
lex AS (
    SELECT s.skill_id, s.embedding <=> sqlc.arg(query_embedding)::vector AS distance
    FROM search_documents s
    JOIN workspaces w ON w.id = s.workspace_id AND w.is_catalog
    WHERE (s.enrichment_status = 'enriched' OR s.embedding IS NOT NULL)
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
       s.tags, s.scan, ver.created_at AS verified_at,
       COALESCE(cmp.capability, 'unverified') AS agent_capability,
       COALESCE(cmp.runtime, 'unverified') AS agent_runtime,
       COALESCE(cmp.runtime_image, '') AS agent_runtime_image,
       cmp.measured_at AS agent_measured_at,
       COALESCE(cur.tier, 'indexed') AS curation_tier,
       cur.category,
       cur.category_source,
       (1 - COALESCE(c.distance, 1))::float8 AS rank,
       (c.distance IS NULL)::bool AS unranked,
       c.covered AS lexical_covered,
       count(*) OVER ()::bigint AS total_matches
FROM candidates c
JOIN search_documents s ON s.skill_id = c.skill_id
LEFT JOIN LATERAL (
    SELECT v.id, v.created_at
    FROM skill_versions v
    WHERE v.skill_id = c.skill_id
    ORDER BY v.version_number DESC
    LIMIT 1
) ver ON true
LEFT JOIN LATERAL (
    SELECT sc.capability, sc.runtime, sc.runtime_image, sc.measured_at
    FROM skill_runtime_compatibility sc
    WHERE sc.skill_version_id = ver.id
    ORDER BY sc.measured_at DESC
    LIMIT 1
) cmp ON true
LEFT JOIN LATERAL (
    SELECT CASE
        WHEN sk.curation_tier = 'curated' AND sk.curated_version_id = ver.id
        THEN 'curated' ELSE 'indexed'
    END AS tier,
    sk.category, sk.category_source
    FROM skills sk
    WHERE sk.id = c.skill_id
) cur ON true
WHERE (c.covered OR c.distance IS NULL OR c.distance <= sqlc.arg(max_distance)::float8)
  AND (
    sqlc.narg(has_script)::bool IS NULL
    OR (s.scan IS NOT NULL
        AND (s.scan->'codes' @> '["script-file"]'::jsonb
             OR s.scan->'codes' @> '["embedded-script"]'::jsonb) = sqlc.narg(has_script)::bool)
  )
  AND (
    sqlc.narg(spec_validated)::bool IS NULL
    OR (ver.created_at IS NOT NULL) = sqlc.narg(spec_validated)::bool
  )
  AND (
    sqlc.narg(agent_runtime)::text IS NULL
    OR COALESCE(cmp.runtime, 'unverified') = sqlc.narg(agent_runtime)::text
  )
  AND (
    sqlc.narg(curation_tier)::text IS NULL
    OR COALESCE(cur.tier, 'indexed') = sqlc.narg(curation_tier)::text
  )
  AND (
    sqlc.narg(category)::text IS NULL
    OR cur.category = sqlc.narg(category)::text
  )
ORDER BY (lower(s.name) = lower(btrim(sqlc.arg(query)::text))) DESC,
         c.covered DESC,
         c.distance ASC NULLS LAST
LIMIT sqlc.arg(result_limit);

-- name: ReindexAll :execrows
INSERT INTO search_documents (skill_id, workspace_id, name, summary, updated_at)
SELECT sk.id, sk.workspace_id, sk.name, coalesce(sk.summary, ''), now()
FROM skills sk
WHERE sk.deleted_at IS NULL AND sk.takedown_at IS NULL
ON CONFLICT (skill_id) DO UPDATE
SET workspace_id = EXCLUDED.workspace_id, name = EXCLUDED.name,
    summary = EXCLUDED.summary;

-- name: DeleteSearchDocument :exec
DELETE FROM search_documents WHERE skill_id = $1 AND workspace_id = $2;

-- name: PruneDeletedSearchDocuments :execrows
DELETE FROM search_documents sd
USING skills sk
WHERE sd.skill_id = sk.id
  AND (sk.deleted_at IS NOT NULL OR sk.takedown_at IS NOT NULL);

-- name: ListSkillScans :many
SELECT skill_id, scan
FROM search_documents
WHERE workspace_id = $1 AND skill_id = ANY(sqlc.arg(skill_ids)::uuid[]);

-- name: ListCatalogSkillScans :many
SELECT sd.skill_id, sd.scan
FROM search_documents sd
JOIN workspaces w ON w.id = sd.workspace_id AND w.is_catalog
WHERE sd.skill_id = ANY(sqlc.arg(skill_ids)::uuid[]);

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
FROM workspaces w, skills sk
WHERE w.id = sd.workspace_id AND w.is_catalog
  AND sk.id = sd.skill_id AND sk.deleted_at IS NULL AND sk.takedown_at IS NULL
  AND sd.enrichment_status = 'enriched'
  AND COALESCE(sd.enrichment_prompt_version, '') <> sqlc.arg(prompt_version)::text;

-- name: GetCatalogReferenceFacts :one
SELECT sd.scan,
       (sk.curation_tier = 'curated' AND sk.curated_version_id = sqlc.arg(version_id)::uuid)::bool AS curated
FROM search_documents sd
JOIN skills sk ON sk.id = sd.skill_id
JOIN workspaces w ON w.id = sd.workspace_id AND w.is_catalog
WHERE sd.skill_id = $1;

-- name: CreationLexicalSearchSkills :many
SELECT s.skill_id, s.name
FROM search_documents s
JOIN workspaces w ON w.id = s.workspace_id AND w.is_catalog
WHERE (s.enrichment_status = 'enriched' OR s.embedding IS NOT NULL)
  AND s.bigram @@ to_tsquery('simple', sqlc.arg(query)::text)
ORDER BY ts_rank_cd(s.bigram, to_tsquery('simple', sqlc.arg(query)::text)) DESC
LIMIT sqlc.arg(result_limit)::int;
