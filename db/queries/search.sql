-- name: UpsertSearchDocument :exec
INSERT INTO search_documents (skill_id, workspace_id, name, summary, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (skill_id) DO UPDATE
SET name = EXCLUDED.name, summary = EXCLUDED.summary, updated_at = now();

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

-- name: ReindexAll :execrows
-- Rebuilds the whole projection from the source of truth (INGEST-009 重新索引).
-- Idempotent; safe to run any time.
INSERT INTO search_documents (skill_id, workspace_id, name, summary, updated_at)
SELECT sk.id, sk.workspace_id, sk.name, coalesce(sk.summary, ''), now()
FROM skills sk
ON CONFLICT (skill_id) DO UPDATE
SET workspace_id = EXCLUDED.workspace_id, name = EXCLUDED.name,
    summary = EXCLUDED.summary, updated_at = now();
