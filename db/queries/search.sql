-- name: UpsertSearchDocument :exec
INSERT INTO search_documents (skill_id, workspace_id, name, summary, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (skill_id) DO UPDATE
SET name = EXCLUDED.name, summary = EXCLUDED.summary, updated_at = now();

-- name: UpsertSearchDocumentWithEmbedding :exec
-- Full upsert including the embedding vector (ADR-013 index-time enhancement).
INSERT INTO search_documents (skill_id, workspace_id, name, summary, embedding, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (skill_id) DO UPDATE
SET name = EXCLUDED.name, summary = EXCLUDED.summary,
    embedding = EXCLUDED.embedding, updated_at = now();

-- name: SearchSkills :many
-- FTS leg only for now (ADR-013); vector + RRF join here when the embedding
-- pipeline lands. websearch_to_tsquery tolerates raw user input.
SELECT s.skill_id, s.workspace_id, s.name, s.summary,
       ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text))::float8 AS rank
FROM search_documents s
WHERE s.workspace_id = $1
  AND s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
ORDER BY rank DESC
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
SELECT s.skill_id, s.name, s.summary,
       ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text))::float8 AS rank
FROM search_documents s
JOIN workspaces w ON w.id = s.workspace_id AND w.is_catalog
WHERE s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
ORDER BY rank DESC
LIMIT $1;

-- name: PublicHybridSearchSkills :many
-- ADR-013 hybrid retrieval: FTS + vector similarity fused with RRF.
-- Both legs run as CTEs; RRF combines their rankings. A leg with no hits
-- contributes nothing (ADR-013 adjustment 3: zero-hit legs excluded).
WITH fts AS (
    SELECT s.skill_id, s.name, s.summary,
           ROW_NUMBER() OVER (ORDER BY ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text)) DESC) AS rn
    FROM search_documents s
    JOIN workspaces w ON w.id = s.workspace_id AND w.is_catalog
    WHERE s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
    LIMIT 50
),
vec AS (
    SELECT s.skill_id, s.name, s.summary,
           ROW_NUMBER() OVER (ORDER BY s.embedding <=> sqlc.arg(query_embedding)::vector ASC) AS rn
    FROM search_documents s
    JOIN workspaces w ON w.id = s.workspace_id AND w.is_catalog
    WHERE s.embedding IS NOT NULL
    LIMIT 50
),
rrf AS (
    SELECT COALESCE(f.skill_id, v.skill_id) AS skill_id,
           COALESCE(f.name, v.name) AS name,
           COALESCE(f.summary, v.summary) AS summary,
           COALESCE(1.0 / (60 + f.rn), 0) + COALESCE(1.0 / (60 + v.rn), 0) AS score
    FROM fts f
    FULL OUTER JOIN vec v ON f.skill_id = v.skill_id
)
SELECT skill_id, name, summary, score::float8 AS rank
FROM rrf
ORDER BY score DESC
LIMIT sqlc.arg(result_limit);

-- name: ReindexAll :execrows
-- Rebuilds the whole projection from the source of truth (INGEST-009 重新索引).
-- Idempotent; safe to run any time.
INSERT INTO search_documents (skill_id, workspace_id, name, summary, updated_at)
SELECT sk.id, sk.workspace_id, sk.name, coalesce(sk.summary, ''), now()
FROM skills sk
WHERE sk.deleted_at IS NULL
ON CONFLICT (skill_id) DO UPDATE
SET workspace_id = EXCLUDED.workspace_id, name = EXCLUDED.name,
    summary = EXCLUDED.summary, updated_at = now();

-- name: DeleteSearchDocument :exec
DELETE FROM search_documents WHERE skill_id = $1;

-- name: PruneDeletedSearchDocuments :execrows
-- Rebuild hygiene: ReindexAll only upserts live skills, so stale documents of
-- soft-deleted skills are removed here first.
DELETE FROM search_documents sd
USING skills sk
WHERE sd.skill_id = sk.id AND sk.deleted_at IS NOT NULL;
