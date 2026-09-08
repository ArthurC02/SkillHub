-- R-25 / OWASP GenAI's LLM01 least-privilege guidance (05, 2026-09-08): the
-- seven `cmd/maintenance` purge subcommands run today under the same database
-- role the API uses for every request, which is why that role holds DELETE on
-- every table any purge ever clears -- including tables no API request
-- handler ever deletes a row from (skill_sources, audit_events, user
-- sessions, the object-collection worklist). A request handler compromised by
-- a prompt-injected instruction (evaluation-2 in
-- docs/plans/mvp/m5/creation-measure/injection/corpus-injection.json is one
-- shape of that) inherits whatever the connection role can do; the fix is not
-- a better prompt, it is a role that cannot do more than clearing needs.
--
-- This migration creates that NOLOGIN role, scoped to exactly the tables the
-- seven subcommands read, write or delete -- verified by reading
-- apps/platform/cmd/maintenance's call chains (identity.PurgeExpiredAccounts,
-- audit.PurgeExpired, analytics.PurgeExpiredFeedback, objreconcile.PurgeExpired
-- via run/testlab's Mark*Purged and Guard* functions, registry.PurgeDeletedSkills,
-- registry.CollectOrphanObjects, ingest.CheckSources), not guessed from table
-- names.
--
-- `rotate-partitions` is deliberately excluded: partition.MaintainPartitions
-- runs CREATE TABLE / DROP TABLE against trace_events' and analytics_events'
-- monthly partitions, which needs schema-level DDL rights, not a table-scoped
-- SELECT/DELETE grant. Folding DDL into this role would defeat the
-- least-privilege point of having it; a deployment that wants that
-- subcommand under a narrow role needs a second, differently-shaped grant.
-- See docs/runbooks/purge-role-cutover.md.
--
-- NOLOGIN: this role is never a connection target by itself. A deployment
-- GRANTs it to whatever login role SKILLHUB_PURGE_DATABASE_URL's connection
-- string authenticates as (cmd/maintenance/main.go), the same way a Unix
-- group is granted to a user rather than logged into directly. No password,
-- no CREATEDB, no BYPASSRLS -- there is nothing here for a connection to do
-- except read and clear the tables listed below.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'skillhub_purge') THEN
        CREATE ROLE skillhub_purge NOLOGIN;
    END IF;
END;
$$;

-- Read access for every candidate list and provenance check the seven
-- subcommands run. Several of these are read-only from this role's point of
-- view -- `runs`, `run_attempts`, `test_case_snapshots`, `download_artifacts`
-- back PurgeUnreferencedSkills'/PurgeSkillsPastDeletionGrace's "may this
-- skill go" exclusions and AccountPurgeReady's sandbox-quiescence check, and
-- nothing here ever deletes a row from them.
GRANT SELECT ON
    users, workspaces, datasets, dataset_object_cleanup_intents,
    test_cases, test_case_snapshots, run_artifact_upload_intents, artifacts,
    download_object_cleanup_intents, download_artifacts, skills, skill_versions,
    runs, run_attempts, object_collection_queue, skill_sources
    TO skillhub_purge;

-- Bookkeeping columns a purge marks in place instead of deleting: the
-- reconcile worklists' retry/attempt timestamps, account deletion's
-- purge-progress and anonymisation flags, and check-sources' availability
-- probe (`maintenance check-sources`, INGEST-010).
GRANT UPDATE ON
    users, workspaces, datasets, dataset_object_cleanup_intents,
    run_artifact_upload_intents, artifacts, download_object_cleanup_intents,
    analytics_events, feedback_reports, skill_sources
    TO skillhub_purge;

-- The two inserts a purge statement makes: PurgeUnreferencedSkills and
-- PurgeSkillsPastDeletionGrace enqueue the package objects they are about to
-- orphan in the same statement as their DELETE (0039, 04 丙-73) -- a second
-- definition of "which skills are going", written later and separately,
-- would drift -- and purgeAccount's transaction records the account-purge
-- tombstone in the audit trail alongside the rows it removes (iron rule 9).
GRANT INSERT ON object_collection_queue, audit_events TO skillhub_purge;

-- The clearing this role exists for: every table a maintenance subcommand's
-- DELETE statement names.
GRANT DELETE ON
    datasets, dataset_object_cleanup_intents, test_cases, run_artifact_upload_intents,
    artifacts, download_object_cleanup_intents, download_records, download_artifacts,
    creation_sessions, skill_versions, skills, skill_sources, user_identities, sessions,
    object_collection_queue, object_reconcile_sightings, audit_events, feedback_reports
    TO skillhub_purge;

-- Why the API role's own DELETE is not revoked here
-- ---------------------------------------------------
-- Every currently-running deployment's API and Worker processes, and
-- cmd/maintenance until an operator points SKILLHUB_PURGE_DATABASE_URL
-- somewhere else, connect as the SAME role this migration is narrowing a
-- copy of. That role's DELETE on these tables is not only what purges use --
-- it is what an owner's own dataset delete, a skill's own soft/hard delete
-- endpoint, and session logout all go through today, on the same grant.
-- Revoking it here, in a migration that runs the moment a deployment
-- upgrades, would remove that ability from every live delete endpoint before
-- any login has been granted skillhub_purge and before cmd/maintenance has
-- been repointed at one -- turning a least-privilege migration into an
-- outage on arrival.
--
-- The revoke is therefore an operator action with a precondition, not a
-- migration statement with a schedule: only after (1)
-- SKILLHUB_PURGE_DATABASE_URL is set for every deployment of `maintenance`
-- that runs a purge subcommand (cron and any manual invocation) and (2) a
-- purge run has been observed succeeding end to end under the new role may
-- the API role's DELETE on these tables be revoked. Doing it earlier moves
-- purge traffic onto skillhub_purge while leaving the API role exactly as
-- over-privileged as before, for no safety gain -- and doing it without (2)
-- risks discovering a missing grant above by breaking production deletes.
-- The exact sequence, verification steps and the revoke statement itself are
-- in docs/runbooks/purge-role-cutover.md; this migration only builds the
-- role the runbook grants.
