-- ROLLOUT: stop every pre-0049 maintenance process and drain every old API
-- object writer before this migration; do not restart either. Old APIs can PUT
-- bytes before their fenced INSERT, which a database trigger cannot protect.
-- the exact drain sequence is docs/runbooks/account-purge-write-fence-rollout.md.
--
-- Serialize every new workspace-owned row with account deletion. A producer
-- takes this shared transaction lock before its INSERT becomes visible; the
-- purge holds the matching exclusive session lock from its final eligibility
-- check through object deletion and the database transaction. This closes the
-- gap left by HTTP authentication: a request may already be in flight when an
-- account becomes due for deletion.

CREATE FUNCTION fence_workspace_write_during_account_purge()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    owner_blocked boolean;
BEGIN
    IF current_setting('skillhub.purge', true) = 'on' OR NEW.workspace_id IS NULL THEN
        RETURN NEW;
    END IF;
    PERFORM pg_advisory_xact_lock_shared(
        hashtextextended('workspace-objects:' || NEW.workspace_id::text, 0));
    SELECT u.deleted_at IS NOT NULL OR u.purge_started_at IS NOT NULL
      INTO owner_blocked
      FROM workspaces w JOIN users u ON u.id = w.owner_user_id
     WHERE w.id = NEW.workspace_id;
    IF coalesce(owner_blocked, true) THEN
        RAISE EXCEPTION 'workspace owner no longer accepts writes'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

DO $$
DECLARE
    table_name text;
BEGIN
    FOREACH table_name IN ARRAY ARRAY[
        'skill_sources', 'skills', 'skill_versions',
        'test_cases', 'datasets', 'test_case_snapshots', 'runs',
        'run_status_transitions', 'trace_events', 'evaluations', 'artifacts',
        'run_attempts', 'outbox_events', 'run_permission_confirmations',
        'evaluation_suggestions', 'download_artifacts', 'download_records',
        'analytics_events', 'feedback_reports', 'evaluation_model_usage',
        'dataset_object_cleanup_intents'
    ] LOOP
        EXECUTE format(
            'CREATE TRIGGER account_purge_insert_fence BEFORE INSERT ON %I '
            'FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge()',
            table_name);
    END LOOP;
END;
$$;

-- These are the mutable private drafts whose contents can change without
-- creating another row. Operational updates (outbox publication, cleanup
-- bookkeeping, state transitions) deliberately remain available.
CREATE TRIGGER account_purge_update_fence
BEFORE UPDATE ON skills
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

CREATE TRIGGER account_purge_update_fence
BEFORE UPDATE ON test_cases
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

CREATE TRIGGER account_purge_update_fence
BEFORE UPDATE ON datasets
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

-- Session creation has no workspace_id column, but it is another in-flight
-- write that must not recreate account-owned state after the purge snapshot.
CREATE FUNCTION fence_session_write_during_account_purge()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    workspace uuid;
    owner_blocked boolean;
BEGIN
    SELECT w.id, u.deleted_at IS NOT NULL OR u.purge_started_at IS NOT NULL
      INTO workspace, owner_blocked
      FROM users u JOIN workspaces w ON w.owner_user_id = u.id
     WHERE u.id = NEW.user_id;
    IF workspace IS NULL THEN
        RAISE EXCEPTION 'session owner has no workspace' USING ERRCODE = '55000';
    END IF;
    PERFORM pg_advisory_xact_lock_shared(
        hashtextextended('workspace-objects:' || workspace::text, 0));
    -- Re-read after taking the lock: an exclusive purge may have won while the
    -- trigger waited.
    SELECT u.deleted_at IS NOT NULL OR u.purge_started_at IS NOT NULL
      INTO owner_blocked FROM users u WHERE u.id = NEW.user_id;
    IF coalesce(owner_blocked, true) THEN
        RAISE EXCEPTION 'session owner is being purged' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON sessions
FOR EACH ROW EXECUTE FUNCTION fence_session_write_during_account_purge();
