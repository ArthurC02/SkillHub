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

-- name: ListWorkspaceAuditEvents :many
-- What happened in this workspace, most recent first (02:GEN-003 「可查」).
--
-- Workspace-scoped and not actor-scoped even though a personal workspace has one
-- actor: iron rule 3 asks the question of the workspace, and the two answers
-- being equal today is a property of the population, not of the query.
--
-- The action filter is a parameter rather than a WHERE clause per caller so that
-- "generation failures" and any later "what happened here" share one query and
-- one index. Passing an empty array returns nothing, which is the safe direction
-- for a caller that forgot to say what it wanted.
SELECT * FROM audit_events
WHERE workspace_id = $1 AND action = ANY(@actions::text[])
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

-- name: ListWorkspaceDatasetObjectKeys :many
-- Private object keys are split by owner. Identity combines these scoped
-- answers before deleting bytes; package objects remain deliberately absent.
SELECT object_key FROM datasets WHERE workspace_id = $1;

-- name: ListWorkspaceRunArtifactObjectKeys :many
SELECT object_key FROM artifacts
WHERE workspace_id = $1 AND kind = 'run_output';

-- name: ListWorkspaceDownloadArtifactObjectKeys :many
SELECT object_key FROM artifacts
WHERE workspace_id = $1 AND kind = 'download_package';

-- name: DeleteWorkspaceDatasets :execrows
DELETE FROM datasets WHERE workspace_id = $1;

-- name: DeleteWorkspaceRunArtifacts :execrows
DELETE FROM artifacts WHERE workspace_id = $1 AND kind = 'run_output';

-- name: DeleteWorkspaceDownloadArtifacts :execrows
DELETE FROM artifacts WHERE workspace_id = $1 AND kind = 'download_package';

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

-- name: LockSkillForOperatorWrite :one
-- The read half of an operator's licensing-hold change (02:SEC-011 追加小節,
-- 0023). FOR UPDATE because the value it returns becomes the "before" side of an
-- audit event: without the lock, two operators acting at once would both record
-- the same before-state and one of the two events would be a lie.
--
-- 2026-08-21 (DDD-031, ADR-035 B 組): both halves are issued by
-- registry.SetAccessRestriction, which returns this row as the before-state.
-- Until then the read half was issued by internal/catalog and only the write
-- half by the owner, so the owner of the column could not tell whether the row
-- had been locked at all. Nothing here changed; who runs it did.
--
-- Deliberately *not* workspace scoped, which makes it one of only two statements
-- in this tree that are not. That is the whole point of the operator role: a
-- catalogue entry lives in the curator's workspace and a fork of it lives in
-- somebody else's, and a licensing hold has to reach both. Iron rule 3 is not
-- weakened by it — the statement returns three identifiers and one reason code,
-- no package content, no name, nothing about the workspace's private contents,
-- and it is reachable only from the operator routes (02:SEC-011「operator 不得
-- 讀取任何 Workspace 私有資料」).
--
-- 2026-08-23: also returns `redistribution`, and the name lost the word
-- "Restriction" with it. Two operator-writable governance columns now share one
-- lock statement rather than having one each — they are the same row, taken for
-- the same reason (the value returned becomes an audit event's before-state),
-- and a second FOR UPDATE of the same row would be a second place to get the
-- scoping wrong. Each writer reads the column it owns off the same result.
SELECT id, workspace_id, access_restriction, redistribution FROM skills
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: SetSkillRedistribution :exec
-- The write half of an operator's redistribution verdict (02:SEC-007, 0027,
-- extended by 0036; `04` 乙-17 / `05` R-3c).
--
-- Until now this column had no write path at all outside import: changing it
-- meant running UPDATE by hand. It is the gate that decides whether the platform
-- will hand a skill's bytes to somebody, and it was the only such gate with no
-- endpoint, no authorization check and no audit event — while
-- access_restriction, which blocks the same download for a weaker reason, had
-- all three. That asymmetry is what this closes; it does not answer who may call
-- it or what evidence they must show (`05` R-3a/R-3b, still open).
--
-- Unscoped by workspace for the same reason as the statements above: a
-- redistribution verdict is about the content, and the same content exists as
-- forks in other people's workspaces.
--
-- The CHECK on the column refuses anything outside the four values; the caller
-- refuses `self_supplied` on top of that, because that value is a fact about who
-- brought the bytes (0036) and not a verdict anyone can assert on their behalf.
UPDATE skills SET redistribution = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SetSkillAccessRestriction :exec
-- The write half. One statement for both directions: a reason code holds the
-- materials, NULL lifts the hold (0023 「旗標可解除」). Two statements would be
-- two places to forget updated_at, and the CHECK on the column already refuses
-- the one value that must not be written (the empty string).
--
-- Unscoped for the same reason as the read above, and narrower than it looks:
-- one column of one row addressed by primary key. Nothing else about the skill
-- is touched — a hold is not a takedown and not an edit of anybody's content.
UPDATE skills SET access_restriction = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListSourcesToCheck :many
-- Least-recently-probed first, so repeated bounded runs sweep the whole table.
-- unavailable_since comes back so the caller can tell a state *change* from a
-- repeat of the same answer: a probe that fails again is not news, the one that
-- first fails, or first succeeds again, is. workspace_id rides along so that
-- event can be scoped to the workspace whose content changed availability.
SELECT id, workspace_id, source_url, unavailable_since FROM skill_sources
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
