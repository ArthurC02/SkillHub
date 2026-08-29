-- Account deletion (CORE-007), audit trail (CORE-008), manual takedown and
-- source availability (INGEST-010).

-- name: InsertAuditEvent :exec
-- Always written with the caller's transaction handle so the event and the
-- domain change commit together (iron rule 9).
INSERT INTO audit_events (actor_user_id, workspace_id, action, resource_type, resource_id, metadata)
VALUES ($1, $2, $3, $4, $5, $6);

-- ListAuditEventsByActor was removed on 2026-08-25: it had no caller, and it
-- carried a hand-written owner override in db/query-owners.yaml maintained for a
-- query nobody called. The 0013 index it read (audit_events_actor_idx) stays:
-- "what did this account do" during an incident is answered at a psql prompt,
-- which is where NFR-001 expects an investigator to be, and that needs the index
-- rather than a Go method.

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

-- name: DeleteExpiredAuditEvents :execrows
-- The 400 day sweep PDM-006 6 promises and the consent document tells a
-- participant about (gate-test/consent-and-data-policy.md 3).
--
-- It did not exist until 2026-08-25. The column comment in 0013 said "400 day
-- retention", the index above it is named for the sweep, and the DELETE branch
-- of enforce_immutable was opened for it -- three pieces of a mechanism whose
-- fourth piece nobody wrote. What a participant was asked to sign said the row
-- disappears after 400 days; what ran said never.
--
-- Requires SET LOCAL skillhub.purge = 'on' in the same transaction: the 0013
-- trigger blocks every other DELETE on this table, which is the point of an
-- audit trail and is not being relaxed here.
DELETE FROM audit_events WHERE created_at < $1;

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
      -- The skill-level half of the same rule. Dropping the `<> workspace_id` on
      -- 2026-08-29 with its version-level sibling below, and measured rather than
      -- assumed: with only the version-level exclusion in place, a same-workspace
      -- fork still fails on skills_forked_from_skill_id_fkey first. The two
      -- columns are written together by registry.Fork today, so either exclusion
      -- alone happens to cover both -- and "happens to" is the word that makes it
      -- worth writing both down.
      AND NOT EXISTS (
            SELECT 1 FROM skills f WHERE f.forked_from_skill_id = sk.id
          )
      -- Test cases carry their own retention and snapshots runs point at; a
      -- skill still holding them is out of scope for this statement.
      --
      -- Deliberately NOT `AND tc.deleted_at IS NULL`, and that is a measured
      -- result rather than an oversight. 0017 soft-deletes test cases and never
      -- removes the row, so as written a test case the user deleted months ago
      -- pins its skill here for the rest of time, and both this sweep and the
      -- grace sweep below skip it forever. That is a real defect. But adding the
      -- predicate on its own does not fix it, it relocates it: test_cases.skill_id
      -- is NOT NULL REFERENCES skills (id) with NO ACTION (0004:9), so the skill
      -- becomes purgeable while its soft-deleted rows still name it and the DELETE
      -- below fails with
      --   23503 ... "skills" violates foreign key "test_cases_skill_id_fkey"
      -- taking the whole account purge down with it. Measured 2026-08-29 against a
      -- real database, not reasoned about.
      --
      -- The missing piece is one layer up: testlab's account-purge step deletes
      -- `datasets` and nothing else (trial/design/purge.go), so test_cases, the
      -- table holding TEST-001's user_prompt and therefore the user's own words,
      -- has no delete path anywhere in this repository. Once that step removes the
      -- soft-deleted rows this predicate needs no change at all, because there
      -- will be no row left for it to find. Whether those rows MAY go is a real
      -- question and not a coding one: a frozen test_case_snapshot may point at
      -- one (0005), and then it has to stay.
      AND NOT EXISTS (SELECT 1 FROM test_cases tc WHERE tc.skill_id = sk.id)
      -- Nothing may be forked from a skill that is about to go, whatever
      -- workspace the fork is in. `referenced` above already spares the
      -- cross-workspace case; this is the same rule for a fork of your own skill,
      -- which Fork allows (registry.Fork reads the caller's own workspace first).
      -- Without it the ancestor's skill_versions rows go while a surviving fork
      -- still names one, and 0003's forked_from_version_id — NO ACTION, and
      -- deliberately not CASCADE, because a cascade would take the fork lineage
      -- with it — raises 23503 and rolls the whole account purge back.
      --
      -- Any fork counts, not only a surviving one. Deciding "the fork is going
      -- too, so the ancestor may go" is self-referential (the fork's own fate is
      -- what this CTE is computing) and would need a recursive fixed point for a
      -- case worth two rows; keeping the ancestor instead is the same treatment
      -- `referenced` already gives, its owner is de-identified either way, and it
      -- cannot fail.
      AND NOT EXISTS (
            SELECT 1 FROM skills f
            JOIN skill_versions v ON v.id = f.forked_from_version_id
            WHERE v.skill_id = sk.id
          )
      -- Same shape for the packaging side: download_artifacts.skill_version_id is
      -- NO ACTION too (0027), so a version somebody packaged cannot be deleted
      -- while its detail row stands. In the account purge packaging's own step has
      -- already removed this workspace's rows by the time this runs, which is why
      -- this line looks redundant there and is not: the grace purge below shares
      -- this predicate and has no such step in front of it.
      AND NOT EXISTS (
            SELECT 1 FROM download_artifacts da
            JOIN skill_versions v ON v.id = da.skill_version_id
            WHERE v.skill_id = sk.id
          )
),
enqueued AS (
    -- The producer for object_collection_queue (0039, 04 丙-73), and it lives
    -- HERE rather than in Go for one reason: it hangs off the same `purgeable`
    -- the DELETE below uses. Recomputing that predicate on the Go side would put
    -- a second definition of "which skills are going" next to the first, and the
    -- day they drift the queue silently misses keys -- which is invisible,
    -- because a key nothing enqueued is a key nothing will ever look for.
    --
    -- Same transaction as the delete, so iron rule 9 holds without ceremony: a
    -- purge that rolls back takes its enqueue with it, and one that commits
    -- cannot have lost it. Reads skill_versions before the sibling CTE deletes
    -- from it -- every CTE sees the statement's snapshot, so the keys are still
    -- there to read.
    --
    -- Candidates, not condemned objects: the key may still belong to a fork.
    -- ListCollectableObjects decides that later, against the table.
    INSERT INTO object_collection_queue (object_key)
    SELECT DISTINCT v.package_object_key
    FROM skill_versions v
    WHERE v.skill_id IN (SELECT id FROM purgeable)
      AND v.package_object_key <> ''
    ON CONFLICT (object_key) DO NOTHING
),
versions AS (
    -- search_documents.skill_id cascades off the skills delete below.
    DELETE FROM skill_versions WHERE skill_id IN (SELECT id FROM purgeable)
)
DELETE FROM skills WHERE id IN (SELECT id FROM purgeable);

-- name: PurgeSkillsPastDeletionGrace :execrows
-- WS-005 / PDM-006 §6.1: the grace purge for a skill the user deleted on its own.
--
-- Until 2026-08-25 this did not exist, and `DELETE /skills/{id}` told the user,
-- verbatim on the screen that confirmed the deletion, that snapshots were
-- "retained for the 30-day grace period, then purged". The only hard delete of a
-- skill in this repo was PurgeUnreferencedSkills, which takes a workspace id, has
-- no deleted_at predicate, and runs from account deletion alone -- so a skill
-- deleted on its own kept deleted_at set and its rows forever (04 丙-63).
--
-- Not workspace scoped, and that is the one structural difference from its
-- sibling above: this is the control plane sweeping its own backlog on a clock,
-- not a user reading their own data (same exemption RUN-008's supervisor scan
-- has). The cutoff is the caller's, because the grace period is deployment
-- configuration that no row carries -- unlike `artifacts.expires_at`, where the
-- deadline is on the row and a caller-supplied window would be a second
-- definition of the same date.
--
-- The three exclusions are PurgeUnreferencedSkills's, and they are not optional
-- politeness: a version somebody else forked or a run used is retained with its
-- owner de-identified instead, because hard deleting it would break a third
-- party's provenance chain (DISC-003) and the immutability rule (iron rule 4) --
-- and it would do so to punish the wrong person. A skill still holding test cases
-- carries their retention, not this one's.
--
-- **The package objects are not removed here**, exactly as the account purge does
-- not remove them: they are content-addressed and shared with every fork, so
-- taking the bytes because one owner deleted their row would break readers who
-- never asked for anything. What this statement does instead is REMEMBER their
-- keys, in the `enqueued` CTE below -- the last moment they are readable at all.
-- `CollectOrphanObjects` takes it from there, and only for keys no version
-- references any more.
--
-- Requires SET LOCAL skillhub.purge = 'on' in the same transaction: skill_versions
-- is frozen by the 0005 trigger and the purge flag is the one exemption (0013).
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
    WHERE sk.deleted_at IS NOT NULL
      AND sk.deleted_at <= @cutoff::timestamptz
      AND NOT EXISTS (SELECT 1 FROM referenced ref WHERE ref.skill_id = sk.id)
      -- The skill-level half of the same rule. Dropping the `<> workspace_id` on
      -- 2026-08-29 with its version-level sibling below, and measured rather than
      -- assumed: with only the version-level exclusion in place, a same-workspace
      -- fork still fails on skills_forked_from_skill_id_fkey first. The two
      -- columns are written together by registry.Fork today, so either exclusion
      -- alone happens to cover both -- and "happens to" is the word that makes it
      -- worth writing both down.
      AND NOT EXISTS (
            SELECT 1 FROM skills f WHERE f.forked_from_skill_id = sk.id
          )
      -- The three exclusions PurgeUnreferencedSkills carries, verbatim and for
      -- the same reasons; the argument for each is written out there.
      AND NOT EXISTS (SELECT 1 FROM test_cases tc WHERE tc.skill_id = sk.id)
      AND NOT EXISTS (
            SELECT 1 FROM skills f
            JOIN skill_versions v ON v.id = f.forked_from_version_id
            WHERE v.skill_id = sk.id
          )
      AND NOT EXISTS (
            SELECT 1 FROM download_artifacts da
            JOIN skill_versions v ON v.id = da.skill_version_id
            WHERE v.skill_id = sk.id
          )
    ORDER BY sk.deleted_at
    LIMIT @row_limit::int
),
enqueued AS (
    -- The producer for object_collection_queue (0039, 04 丙-73), and it lives
    -- HERE rather than in Go for one reason: it hangs off the same `purgeable`
    -- the DELETE below uses. Recomputing that predicate on the Go side would put
    -- a second definition of "which skills are going" next to the first, and the
    -- day they drift the queue silently misses keys -- which is invisible,
    -- because a key nothing enqueued is a key nothing will ever look for.
    --
    -- Same transaction as the delete, so iron rule 9 holds without ceremony: a
    -- purge that rolls back takes its enqueue with it, and one that commits
    -- cannot have lost it. Reads skill_versions before the sibling CTE deletes
    -- from it -- every CTE sees the statement's snapshot, so the keys are still
    -- there to read.
    --
    -- Candidates, not condemned objects: the key may still belong to a fork.
    -- ListCollectableObjects decides that later, against the table.
    INSERT INTO object_collection_queue (object_key)
    SELECT DISTINCT v.package_object_key
    FROM skill_versions v
    WHERE v.skill_id IN (SELECT id FROM purgeable)
      AND v.package_object_key <> ''
    ON CONFLICT (object_key) DO NOTHING
),
versions AS (
    -- search_documents.skill_id cascades off the skills delete below. The row is
    -- already out of every read path (Delete removes it in its own transaction),
    -- so this is the bytes going, not the visibility.
    DELETE FROM skill_versions WHERE skill_id IN (SELECT id FROM purgeable)
)
DELETE FROM skills WHERE id IN (SELECT id FROM purgeable);

-- name: CountSkillsAwaitingDeletionGrace :one
-- How many rows the sweep above is holding but cannot take yet, split by why.
-- Not decoration: the two numbers answer different questions when somebody asks
-- "did the purge work". `waiting` shrinking to zero is the job draining; `kept`
-- never shrinking is correct and is the provenance rule doing its work, so an
-- operator who sees a stubborn non-zero total needs to be able to tell which one
-- they are looking at without reading this file.
SELECT
    count(*) FILTER (
        WHERE sk.deleted_at > @cutoff::timestamptz
    )::bigint AS waiting,
    count(*) FILTER (
        WHERE sk.deleted_at <= @cutoff::timestamptz
          AND (
            -- One predicate per exclusion in `purgeable` above, negated. They are
            -- written out rather than shared because a CTE cannot be, and the two
            -- have to be read side by side: a reason that keeps a skill from the
            -- sweep and is missing here makes that skill count as neither
            -- `waiting` nor `kept`, and the operator sees a total that does not
            -- add up on the exact day they are asking why the purge is stuck.
            EXISTS (SELECT 1 FROM skill_versions v
                    WHERE v.skill_id = sk.id
                      AND (EXISTS (SELECT 1 FROM skills f
                                   WHERE f.forked_from_version_id = v.id)
                           OR EXISTS (SELECT 1 FROM runs r WHERE r.skill_version_id = v.id)
                           OR EXISTS (SELECT 1 FROM download_artifacts da
                                      WHERE da.skill_version_id = v.id)))
            OR EXISTS (SELECT 1 FROM skills f WHERE f.forked_from_skill_id = sk.id)
            OR EXISTS (SELECT 1 FROM test_cases tc WHERE tc.skill_id = sk.id)
          )
    )::bigint AS kept
FROM skills sk
WHERE sk.deleted_at IS NOT NULL;

-- name: ListCollectableObjects :many
-- Queue entries whose key no longer appears on any skill_version anywhere.
--
-- The NOT EXISTS is the whole safety property, and it is checked at sweep time
-- rather than at enqueue time on purpose: content-addressed keys can come BACK.
-- Re-importing identical bytes creates a new version with the same key, and a
-- decision made when the row was enqueued would delete an object that is being
-- read again by then.
--
-- Not workspace scoped: a key is bytes, and the question "does any version still
-- reference this" has no owner. That is also why 0039 stores no workspace id.
SELECT object_key FROM object_collection_queue q
WHERE NOT EXISTS (
    SELECT 1 FROM skill_versions v WHERE v.package_object_key = q.object_key
)
ORDER BY enqueued_at
LIMIT @row_limit::int;

-- name: DeleteObjectCollectionEntry :exec
-- The bytes are gone, or the key came back and there is nothing to collect.
-- Idempotent by predicate, so a sweep that died after Remove and before this can
-- simply be re-run.
DELETE FROM object_collection_queue WHERE object_key = $1;

-- name: DropReferencedCollectionEntries :execrows
-- Queue entries whose key is referenced again. Without this they would sit on the
-- worklist forever being skipped, and a worklist that never shrinks is
-- indistinguishable from a sweep that has stopped working — the same failure the
-- token ceiling's fail-open counter exists to make visible.
DELETE FROM object_collection_queue q
WHERE EXISTS (
    SELECT 1 FROM skill_versions v WHERE v.package_object_key = q.object_key
);

-- name: CountCollectableObjects :one
-- The worklist's depth, for the same reason the deletion sweep reports two
-- numbers: an operator needs to tell "draining" from "stuck".
SELECT count(*)::bigint FROM object_collection_queue;

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
--
-- 2026-08-28: `takedown_at` joins them, and it is read for a second reason as
-- well as the first. As a before-state it is what an operator takedown records;
-- as a precondition it is how SetSkillTakedown tells "already down" from "not
-- there", which the workspace-scoped path answers with a second scoped read it
-- cannot make here (there is no workspace to scope to). One statement, two
-- questions, still one lock.
SELECT id, workspace_id, access_restriction, redistribution, takedown_at FROM skills
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

-- name: SetSkillTakedown :exec
-- The platform-wide half of INGEST-010 人工下架 (02:SEC-011 動作 ①).
--
-- This is the second query registry.go's ponytail note asked for, written the
-- way it asked: "a second query without the workspace predicate behind that
-- role, not a change to this one". TakedownSkill above is unchanged and still
-- carries `workspace_id = $2`, because an owner taking down their own content
-- is a different authorization from an operator answering an abuse report.
--
-- Until this existed, a DMCA notice or an abuse report about content in
-- somebody else's workspace had no path at all — registry.go said so in a
-- comment and nothing else did. 02:SEC-011 has specified the actor since
-- 2026-08-16; what was missing was one statement.
--
-- Unscoped for the same reason as the three statements above, and it writes the
-- same two columns the scoped path writes. 02:533 forbids a second takedown
-- flow for operators, and this is why that holds structurally rather than by
-- discipline: same `takedown_at`, so the same 410 Gone and the same search
-- exclusion answer it, whoever set it.
--
-- The `takedown_at IS NULL` guard stays, so a second takedown of the same skill
-- affects no row. The caller has already read that column under the lock and
-- refuses before reaching here; the guard is the belt to that pair of braces.
UPDATE skills SET takedown_at = now(), takedown_reason = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL AND takedown_at IS NULL;

-- name: ListSourcesToCheck :many
-- Least-recently-probed first, so repeated bounded runs sweep the whole table.
-- unavailable_since comes back so the caller can tell a state *change* from a
-- repeat of the same answer: a probe that fails again is not news, the one that
-- first fails, or first succeeds again, is. workspace_id rides along so that
-- event can be scoped to the workspace whose content changed availability.
-- content_hash and content_changed_at ride along for the second question this
-- sweep answers (0041): the stored hash is what a re-fetch is compared against,
-- and the timestamp is how the caller tells the first sweep that saw a
-- difference from every sweep after it.
SELECT id, workspace_id, source_url, unavailable_since, content_hash, content_changed_at
FROM skill_sources
WHERE source_type = 'git' AND source_url IS NOT NULL
ORDER BY last_checked_at NULLS FIRST
LIMIT $1;

-- name: MarkSourceChecked :exec
-- unavailable_since records when the source *started* failing, not the latest
-- failure, so "gone for two weeks" stays distinguishable from a blip. A source
-- that answers again clears it.
--
-- content_changed_at is the opposite shape and that is deliberate: it is set on
-- the first sweep that hashes differently and never cleared. content_hash is the
-- snapshot this workspace holds and iron rule 4 makes it immutable, so upstream
-- coming back is not an event this row can express -- only a re-import is, and a
-- re-import writes a new row.
UPDATE skill_sources
SET last_checked_at = now(),
    unavailable_since = CASE
        WHEN sqlc.arg(available)::bool THEN NULL
        ELSE coalesce(unavailable_since, now())
    END,
    content_changed_at = CASE
        WHEN sqlc.arg(content_changed)::bool THEN coalesce(content_changed_at, now())
        ELSE content_changed_at
    END
WHERE id = $1;
