-- name: UpsertSearchDocument :exec
INSERT INTO search_documents (skill_id, workspace_id, name, summary, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (skill_id) DO UPDATE
SET name = EXCLUDED.name, summary = EXCLUDED.summary, updated_at = now();

-- name: UpsertSearchDocumentEnriched :exec
-- Full upsert including the ADR-013 index-time enhancement fields and the
-- embedding computed from them.
--
-- A failed enrichment overwrites a previous good one with empty text and
-- 'pending' on purpose. The old enrichment described the old content; keeping it
-- against new content would index the document as something it no longer is,
-- which is worse than indexing it as pending until the backfill catches up.
INSERT INTO search_documents (
    skill_id, workspace_id, name, summary,
    enriched_summary, task_examples, tags, limitations, scan, embedding,
    enrichment_status, enrichment_model, enrichment_prompt_version, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now())
ON CONFLICT (skill_id) DO UPDATE
SET workspace_id = EXCLUDED.workspace_id,
    name = EXCLUDED.name,
    summary = EXCLUDED.summary,
    enriched_summary = EXCLUDED.enriched_summary,
    task_examples = EXCLUDED.task_examples,
    tags = EXCLUDED.tags,
    limitations = EXCLUDED.limitations,
    scan = EXCLUDED.scan,
    embedding = EXCLUDED.embedding,
    enrichment_status = EXCLUDED.enrichment_status,
    enrichment_model = EXCLUDED.enrichment_model,
    enrichment_prompt_version = EXCLUDED.enrichment_prompt_version,
    updated_at = now();

-- name: ListPendingEnrichment :many
-- Backfill worklist for cmd/reindex: documents whose enrichment never landed,
-- oldest first, with the package object the enrichment is recomputed from.
--
-- The lateral join is an inner join on purpose: a skill with no version yet
-- (a fork created ahead of its content) has nothing to enrich from, so it drops
-- out of the worklist here rather than becoming a null the caller has to skip.
SELECT sd.skill_id, sd.workspace_id, sd.name, sv.package_object_key
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
ORDER BY sd.updated_at
LIMIT $1;

-- name: SearchSkills :many
-- FTS leg only for now (ADR-013); vector + RRF join here when the embedding
-- pipeline lands. websearch_to_tsquery tolerates raw user input.
--
-- The lexical score orders the page and is not selected; see PublicSearchSkills
-- for why it must not be handed to a caller as a rank.
SELECT s.skill_id, s.workspace_id, s.name, s.summary
FROM search_documents s
WHERE s.workspace_id = $1
  AND s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
ORDER BY ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text)) DESC
LIMIT $2;

-- The two queries below serve unauthenticated callers (DISC-001), so their
-- scope cannot come from a session. They are restricted to catalog workspaces
-- (0010) instead of taking a workspace argument: a public query that accepts a
-- caller-supplied scope is exactly the shape iron rule 3 forbids, and a public
-- query with no scope at all leaks every private fork. "Public" is spelled out
-- in the name so no future caller reaches for one of these on a private path.

-- name: PublicSearchSkills :many
-- FTS-only public search — the degradation path when the embedding service is
-- unavailable (ADR-013 fallback).
--
-- ts_rank_cd orders the page but is not selected. It is an unbounded lexical
-- score, not a cosine similarity, and the two used to arrive at the caller
-- through the same `rank` field that the contract documents as 0..1 — a live
-- answer came back with 1.4. The ordering is what the score is good for, and
-- the array already carries that.
SELECT s.skill_id, s.name,
       COALESCE(NULLIF(s.enriched_summary, ''), s.summary) AS summary,
       s.tags, s.scan, ver.created_at AS verified_at
FROM search_documents s
JOIN workspaces w ON w.id = s.workspace_id AND w.is_catalog
LEFT JOIN LATERAL (
    SELECT v.created_at
    FROM skill_versions v
    WHERE v.skill_id = s.skill_id
    ORDER BY v.version_number DESC
    LIMIT 1
) ver ON true
WHERE s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
ORDER BY ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text)) DESC
LIMIT $1;

-- name: PublicHybridSearchSkills :many
-- ADR-013 hybrid retrieval, ranked by vector distance alone.
--
-- This used to fuse both legs with equal-weight RRF. golden-query-set.md §3.7
-- measured that fusion costing 11 of 48 queries their Top-1 and 5 their
-- recall@5 against the vector leg on its own, because the BM25 leg answers only
-- 20% of Traditional Chinese queries correctly and equal-weight RRF averages
-- that near-dead leg's ranks into the strong one. ADR-013 定案調整 3 already
-- says RRF is a recall-coverage device and not a source of ranking quality, so
-- the legs now do exactly that and no more:
--
--   * vec  — nearest neighbours, and the ranking authority.
--   * fts  — candidate expansion only. It pulls in documents the vector leg
--            missed; those documents are then ranked by their own vector
--            distance like everyone else, never by their lexical rank.
--
-- A zero-hit leg contributes no rows, so it cannot dilute the other one
-- (ADR-013 定案調整 3) — that property survives the switch from FULL OUTER JOIN
-- to UNION, and UNION is now enough because there is no per-leg rank to merge.
--
-- Documents with no embedding yet (enrichment_status = 'pending') have a NULL
-- distance. They can only arrive through the FTS leg, they sort last, and the
-- distance cut-off cannot judge them, so they are kept: dropping them would
-- silently hide every not-yet-enriched skill from search instead of ranking it
-- low.
--
-- The per-leg ORDER BY before LIMIT is load-bearing: LIMIT without ORDER BY is
-- not defined to keep the best rows.
-- The legs carry only the id and the distance: everything displayed is read
-- back from search_documents in the final SELECT, so the DISC-002 result
-- columns are written out once instead of three times.
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
    WHERE s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
    ORDER BY ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text)) DESC
    LIMIT 50
),
candidates AS (
    SELECT skill_id, distance FROM vec
    UNION
    SELECT skill_id, distance FROM fts
)
-- max_distance is the DISC-005 cut-off; see catalog.MaxCosineDistance for the
-- value's derivation and its expiry conditions.
--
-- unranked marks the NULL-distance rows. `rank` still comes back COALESCEd
-- because the caller drops it for exactly those rows and reports a null rank
-- (a lexical-only hit was never measured against the query, and 0 would read
-- as "measured, and terrible"). Keeping the COALESCE means one non-null float
-- column instead of a nullability inference that has to hold across a UNION.
--
-- verified_at is the newest version's creation time: the import that scanned
-- the content. Immutable, so it cannot drift from what it describes. NULL for a
-- skill with no version yet, which is also what makes spec_validation
-- unverified for that row (a blocked package never gets a version).
SELECT c.skill_id, s.name,
       COALESCE(NULLIF(s.enriched_summary, ''), s.summary) AS summary,
       s.tags, s.scan, ver.created_at AS verified_at,
       (1 - COALESCE(c.distance, 1))::float8 AS rank,
       (c.distance IS NULL)::bool AS unranked
FROM candidates c
JOIN search_documents s ON s.skill_id = c.skill_id
LEFT JOIN LATERAL (
    SELECT v.created_at
    FROM skill_versions v
    WHERE v.skill_id = c.skill_id
    ORDER BY v.version_number DESC
    LIMIT 1
) ver ON true
WHERE c.distance IS NULL OR c.distance <= sqlc.arg(max_distance)::float8
ORDER BY c.distance ASC NULLS LAST
LIMIT sqlc.arg(result_limit);

-- name: ReindexAll :execrows
-- Rebuilds the whole projection from the source of truth (INGEST-009 重新索引).
-- Idempotent; safe to run any time.
INSERT INTO search_documents (skill_id, workspace_id, name, summary, updated_at)
SELECT sk.id, sk.workspace_id, sk.name, coalesce(sk.summary, ''), now()
FROM skills sk
WHERE sk.deleted_at IS NULL AND sk.takedown_at IS NULL
ON CONFLICT (skill_id) DO UPDATE
SET workspace_id = EXCLUDED.workspace_id, name = EXCLUDED.name,
    summary = EXCLUDED.summary, updated_at = now();

-- name: DeleteSearchDocument :exec
DELETE FROM search_documents WHERE skill_id = $1;

-- name: PruneDeletedSearchDocuments :execrows
-- Rebuild hygiene: ReindexAll only upserts live skills, so stale documents of
-- soft-deleted and manually taken-down skills (INGEST-010) are removed here
-- first. A rebuild that re-listed taken-down content would undo the takedown.
DELETE FROM search_documents sd
USING skills sk
WHERE sd.skill_id = sk.id
  AND (sk.deleted_at IS NOT NULL OR sk.takedown_at IS NOT NULL);
