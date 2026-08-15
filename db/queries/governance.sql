-- Account deletion (CORE-007), audit trail (CORE-008), manual takedown and
-- source availability (INGEST-010).

-- name: InsertAuditEvent :exec
-- Always written with the caller's transaction handle so the event and the
-- domain change commit together (iron rule 9).
INSERT INTO audit_events (actor_user_id, workspace_id, action, resource_type, resource_id, metadata)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListAuditEventsByActor :many
SELECT * FROM audit_events
WHERE actor_user_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: RequestAccountDeletion :one
-- Starts (or re-reads) the 30-day grace period. Idempotent: asking twice keeps
-- the original start time rather than extending the wait.
UPDATE users
SET deletion_requested_at = coalesce(deletion_requested_at, now()), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: CancelAccountDeletion :one
-- Cancellable for the whole grace period (PDM-006 6.1 避免誤刪不可逆).
UPDATE users SET deletion_requested_at = NULL, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: ListAccountsPastGrace :many
-- Purge worklist. The cutoff is computed by the caller so a test can run the
-- purge without waiting 30 days, and so shortening the policy applies to
-- accounts that already requested deletion.
SELECT id FROM users
WHERE deleted_at IS NULL
  AND deletion_requested_at IS NOT NULL
  AND deletion_requested_at <= sqlc.arg(cutoff)
ORDER BY deletion_requested_at
LIMIT $1;

-- name: ListWorkspaceObjectKeys :many
-- Private uploaded content of one workspace: dataset files and run/download
-- artifacts (PDM-006 6.1 硬刪除). Package objects are deliberately absent —
-- they are content-addressed and shared with every fork, so removing one would
-- break a third party's skill rather than erase this account's data.
SELECT d.object_key FROM datasets d WHERE d.workspace_id = sqlc.arg(workspace_id)
UNION
SELECT a.object_key FROM artifacts a WHERE a.workspace_id = sqlc.arg(workspace_id);

-- name: DeleteWorkspaceDatasets :execrows
DELETE FROM datasets WHERE workspace_id = $1;

-- name: DeleteWorkspaceArtifacts :execrows
DELETE FROM artifacts WHERE workspace_id = $1;

-- name: PurgeUnreferencedSkills :execrows
-- Hard-deletes the workspace's skills that nothing outside it depends on, with
-- their versions, and returns how many skills went.
--
-- A skill is *kept* — content intact, owner de-identified — as soon as another
-- workspace forked it or a run used one of its versions. Deleting those would
-- break a third party's provenance chain (DISC-003 "任何 Skill Hub 修改後的版本
-- 都能追溯到原始來源及 Fork 關係") and the immutability rule (iron rule 4);
-- the privacy the account owner asked for is delivered by de-identifying the
-- users/workspaces rows instead (PDM-006 6.1).
--
-- Requires SET LOCAL skillhub.purge = 'on' in the same transaction: skill_versions
-- is frozen by the 0005 trigger and this is the one exemption (0013).
WITH referenced AS (
    SELECT DISTINCT v.skill_id
    FROM skill_versions v
    WHERE EXISTS (
            SELECT 1 FROM skills f
            WHERE f.forked_from_version_id = v.id AND f.workspace_id <> v.workspace_id
          )
       OR EXISTS (SELECT 1 FROM runs r WHERE r.skill_version_id = v.id)
),
purgeable AS (
    SELECT sk.id FROM skills sk
    WHERE sk.workspace_id = $1
      AND NOT EXISTS (SELECT 1 FROM referenced ref WHERE ref.skill_id = sk.id)
      AND NOT EXISTS (
            SELECT 1 FROM skills f
            WHERE f.forked_from_skill_id = sk.id AND f.workspace_id <> sk.workspace_id
          )
      -- Test cases carry their own retention and snapshots runs point at; a
      -- skill still holding them is out of scope for this statement.
      AND NOT EXISTS (SELECT 1 FROM test_cases tc WHERE tc.skill_id = sk.id)
),
versions AS (
    -- search_documents.skill_id cascades off the skills delete below.
    DELETE FROM skill_versions WHERE skill_id IN (SELECT id FROM purgeable)
)
DELETE FROM skills WHERE id IN (SELECT id FROM purgeable);

-- name: PurgeUnreferencedSkillSources :execrows
-- Import provenance of versions that are gone. Sources still backing a retained
-- version stay, because that version is still readable by whoever forked it.
DELETE FROM skill_sources s
WHERE s.workspace_id = $1
  AND NOT EXISTS (SELECT 1 FROM skill_versions v WHERE v.source_id = s.id);

-- name: DeleteUserIdentities :execrows
DELETE FROM user_identities WHERE user_id = $1;

-- name: DeleteUserSessions :execrows
DELETE FROM sessions WHERE user_id = $1;

-- name: AnonymizeWorkspacesByOwner :execrows
-- The workspace row survives (retained versions and runs reference it) but stops
-- carrying the GitHub login it was named after.
UPDATE workspaces SET name = 'deleted-workspace', updated_at = now()
WHERE owner_user_id = $1;

-- name: AnonymizeUser :one
-- The tombstone (PDM-006 6.1 去識別化保留). Retained content still points here,
-- and nothing here names the person. deleted_at is set last: it is what makes
-- the account gone for every existing read and for the purge worklist, so the
-- whole purge is idempotent by virtue of this one write.
UPDATE users
SET email = 'deleted-' || id::text || '@deleted.invalid',
    display_name = 'Deleted user',
    deleted_at = now(),
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: TakedownSkill :one
-- INGEST-010 人工下架. Content is retained (PDM-006 keeps skills and versions
-- permanently); the caller removes the search document in the same transaction.
UPDATE skills
SET takedown_at = now(), takedown_reason = sqlc.arg(reason), updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL AND takedown_at IS NULL
RETURNING *;

-- name: ListSourcesToCheck :many
-- Least-recently-probed first, so repeated bounded runs sweep the whole table.
SELECT id, source_url FROM skill_sources
WHERE source_type = 'git' AND source_url IS NOT NULL
ORDER BY last_checked_at NULLS FIRST
LIMIT $1;

-- name: MarkSourceChecked :exec
-- unavailable_since records when the source *started* failing, not the latest
-- failure, so "gone for two weeks" stays distinguishable from a blip. A source
-- that answers again clears it.
UPDATE skill_sources
SET last_checked_at = now(),
    unavailable_since = CASE
        WHEN sqlc.arg(available)::bool THEN NULL
        ELSE coalesce(unavailable_since, now())
    END
WHERE id = $1;
