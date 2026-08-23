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
--
-- `redistribution <> 'generated'` keeps generated packages off the worklist
-- (GEN-007). Their documents exist and must — the workspace's own list reads the
-- static-scan facts out of them, and 02:GEN-003 forbids showing a generated
-- package one warning fewer than an imported one. What they never get is the
-- enrichment, because enrichment exists to make a document findable and this one
-- is never searched. Without this predicate the backfill would re-enrich them
-- forever: import already skips the call, so they stay `pending` by design.
JOIN skills sk ON sk.id = sd.skill_id AND sk.deleted_at IS NULL AND sk.takedown_at IS NULL
    AND sk.redistribution <> 'generated'
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
--
-- The join is GEN-007's enforcement point, and it is on the READ side on
-- purpose. A generated skill must not be found by search — including by the
-- person who generated it — but its search_documents row still has to exist,
-- because the workspace's own Skill list reads the static-scan facts out of it
-- and 02:GEN-003 forbids a generated package disclosing one warning fewer than
-- an imported one. Excluding it at write time would have bought the guarantee by
-- deleting the disclosure.
--
-- The public queries below need no equivalent: they are restricted to catalog
-- workspaces, and generation is refused in one (skill/admission/generate.go).
SELECT s.skill_id, s.workspace_id, s.name, s.summary
FROM search_documents s
JOIN skills sk ON sk.id = s.skill_id AND sk.redistribution <> 'generated'
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

-- DISC-003 structured filters, shared by both public queries below.
--
-- Only two of the six dimensions 02:DISC-002 lists have per-row data in M1, so
-- only those two are predicates here. The other four are not silently ignored:
-- they are reported as unavailable by the API and disabled in the UI, because a
-- filter that accepts a value and does not narrow anything is worse than one
-- that says it cannot (see catalog/http.go filterAvailability).
--
--   * has_script   — evidence from the projected import scan. `script-file` is a
--                    script in the package tree, `embedded-script` is runnable
--                    code inside SKILL.md itself (SKILL-003); a user asking for
--                    "contains a script" means either.
--                    Rows with NULL scan match NEITHER true nor false: no scan
--                    was ever projected for them, and answering "no script" for
--                    an unscanned row is exactly the 不得自行推定為通過 that
--                    02:DISC-004 forbids. They drop out of a filtered page and
--                    reappear when the filter is cleared.
--   * spec_validated — a saved version is the evidence, for the reason
--                    0015_search_result_facets.sql gives: skillpkg.Validate
--                    blocks the import on any error-level finding, so a stored
--                    version means static validation passed. No version means
--                    nothing was ever validated, which is unverified and never
--                    "failed".
--   * agent_runtime — 0022's measured verdict for the newest version, matched
--                    exactly rather than as a boolean. `native`/`transpiled`/
--                    `failed` are three answers, not two, and a row with no
--                    measurement is `unverified` — a fourth. A *bool would have
--                    made "not native" quietly include the unmeasured rows,
--                    which is the same 推定 the has_script note refuses.
--
-- All three are sqlc.narg: NULL = dimension not filtered, which is the default.
-- The predicates are written twice rather than factored into a SQL function —
-- two copies of four lines beat a migration for a function that would then need
-- its own drift check.
--
-- The compatibility lateral takes the newest row for the version whatever image
-- it was measured on, and hands the image back with it. The alternative — filter
-- to a configured "current" image — needs a deployment setting to decide which
-- verdict the public catalogue shows, and a wrong setting there would be silent.
-- Labelling the answer with the image it came from cannot be silently wrong.

-- name: PublicSearchSkills :many
-- FTS-only public search — the degradation path when the embedding service is
-- unavailable (ADR-013 fallback).
--
-- The DISC-003 filters apply here too. A degraded answer is already lower
-- recall; letting it also ignore the user's filters would make the page lie
-- about what it contains, and the filter dimensions are projection columns that
-- do not depend on the embedding leg being up.
--
-- ts_rank_cd orders the page but is not selected. It is an unbounded lexical
-- score, not a cosine similarity, and the two used to arrive at the caller
-- through the same `rank` field that the contract documents as 0..1 — a live
-- answer came back with 1.4. The ordering is what the score is good for, and
-- the array already carries that.
SELECT s.skill_id, s.name,
       COALESCE(NULLIF(s.enriched_summary, ''), s.summary) AS summary,
       -- Which branch the COALESCE above took. ADR-013 requires model-written
       -- copy to be labelled, and this row already labels `match_reason` while
       -- printing the model's rewrite of the summary in the same <p> the author's
       -- own text would occupy. Derived here rather than in Go so the flag cannot
       -- disagree with the value it describes.
       CASE WHEN NULLIF(s.enriched_summary, '') IS NULL THEN 'package' ELSE 'model' END
           AS summary_source,
       s.tags, s.scan, ver.created_at AS verified_at,
       -- COALESCEd here rather than in Go: a row with no measurement is
       -- unverified on both axes, which is the same answer the handler used to
       -- hard-code, and sqlc cannot see that an outer-joined column is nullable
       -- (it reads the table's NOT NULL and would generate a scan that panics on
       -- the first unmeasured skill).
       COALESCE(cmp.capability, 'unverified') AS agent_capability,
       COALESCE(cmp.runtime, 'unverified') AS agent_runtime,
       COALESCE(cmp.runtime_image, '') AS agent_runtime_image,
       cmp.measured_at AS agent_measured_at
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
WHERE s.tsv @@ websearch_to_tsquery('english', sqlc.arg(query)::text)
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
ORDER BY ts_rank_cd(s.tsv, websearch_to_tsquery('english', sqlc.arg(query)::text)) DESC
LIMIT sqlc.arg(result_limit);

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
       -- Which branch the COALESCE above took. ADR-013 requires model-written
       -- copy to be labelled, and this row already labels `match_reason` while
       -- printing the model's rewrite of the summary in the same <p> the author's
       -- own text would occupy. Derived here rather than in Go so the flag cannot
       -- disagree with the value it describes.
       CASE WHEN NULLIF(s.enriched_summary, '') IS NULL THEN 'package' ELSE 'model' END
           AS summary_source,
       s.tags, s.scan, ver.created_at AS verified_at,
       COALESCE(cmp.capability, 'unverified') AS agent_capability,
       COALESCE(cmp.runtime, 'unverified') AS agent_runtime,
       COALESCE(cmp.runtime_image, '') AS agent_runtime_image,
       cmp.measured_at AS agent_measured_at,
       (1 - COALESCE(c.distance, 1))::float8 AS rank,
       (c.distance IS NULL)::bool AS unranked
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
WHERE (c.distance IS NULL OR c.distance <= sqlc.arg(max_distance)::float8)
  -- DISC-003 filters, applied after candidate generation.
  --
  -- ponytail: the two legs still take their own 50 rows before this runs, so a
  -- filter that matches only rows 51+ of a leg cannot see them. The catalogue is
  -- 45 documents, so no candidate is currently unreachable; push the predicates
  -- into the two CTEs once the catalogue outgrows the candidate window.
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
ORDER BY c.distance ASC NULLS LAST
LIMIT sqlc.arg(result_limit);

-- name: ReindexAll :execrows
-- Rebuilds the whole projection from the source of truth (INGEST-009 重新索引).
-- Idempotent; safe to run any time.
--
-- `updated_at` is set on insert and left alone on conflict, on purpose. Its only
-- reader is ListPendingEnrichment's "oldest first" ordering, so it means "how
-- long has this document been waiting", and a rebuild does not make a document
-- newer - it re-derives the same two columns from the same source row.
--
-- Stamping now() on every row cost the timestamp its only job: after a rebuild
-- the whole projection shared one instant, ListPendingEnrichment's order became
-- arbitrary, and neither it nor REINDEX_BATCH could keep a backfill away from
-- the 45 fork documents the M2 baseline run left pending in a scratch workspace
-- (~$2 of flagship enrichment per re-run). The workaround was to hand-mark those
-- rows `enriched` before every backfill and restore them after. With the
-- timestamp preserved, the freshly forked documents sort last and a bounded
-- REINDEX_BATCH reaches the genuinely old pending rows first, with no hand step.
INSERT INTO search_documents (skill_id, workspace_id, name, summary, updated_at)
SELECT sk.id, sk.workspace_id, sk.name, coalesce(sk.summary, ''), now()
FROM skills sk
WHERE sk.deleted_at IS NULL AND sk.takedown_at IS NULL
ON CONFLICT (skill_id) DO UPDATE
SET workspace_id = EXCLUDED.workspace_id, name = EXCLUDED.name,
    summary = EXCLUDED.summary;

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

-- name: ListSkillScans :many
-- The projected scan for a set of skills in one workspace, so a caller holding a
-- page of skills can ask once instead of per row.
--
-- Workspace scoped even though skill_id is a primary key: the ids arrive from
-- another context's page, and an unscoped read keyed on caller-supplied ids is
-- the cross-tenant read iron rule 3 exists to stop.
--
-- Rows with no document, and rows whose document has no scan, simply do not come
-- back with a scan — the caller fills both as "unavailable", never as clean
-- (DISC-004 不得自行推定為通過). The commonest of those is a fork: catalog's
-- IndexSkill writes name and summary only, because a fork shares its source's
-- bytes and has nothing of its own to scan.
SELECT skill_id, scan
FROM search_documents
WHERE workspace_id = $1 AND skill_id = ANY(sqlc.arg(skill_ids)::uuid[]);

-- name: ListCatalogSkillScans :many
-- The same read as ListSkillScans, for the public catalogue instead of one
-- workspace. Its only caller is the inherited-measurement path: a fork whose
-- bytes are identical to a catalogue ancestor's shows the ancestor's scan
-- (ADR-042 決策 6).
--
-- Takes no workspace argument on purpose, exactly like GetCatalogSkill: the
-- scope is baked into the statement so a caller cannot name a wider one (鐵律
-- 3). Skills outside the catalogue never match, so a private ancestor stays
-- invisible and the caller reports 未測量 rather than reaching into another
-- workspace to answer.
SELECT sd.skill_id, sd.scan
FROM search_documents sd
JOIN workspaces w ON w.id = sd.workspace_id AND w.is_catalog
WHERE sd.skill_id = ANY(sqlc.arg(skill_ids)::uuid[]);
