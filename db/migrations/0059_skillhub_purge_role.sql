DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'skillhub_purge') THEN
        CREATE ROLE skillhub_purge NOLOGIN;
    END IF;
END;
$$;

GRANT SELECT ON
    users, workspaces, datasets, dataset_object_cleanup_intents,
    test_cases, test_case_snapshots, run_artifact_upload_intents, artifacts,
    download_object_cleanup_intents, download_artifacts, skills, skill_versions,
    runs, run_attempts, object_collection_queue, skill_sources
    TO skillhub_purge;

GRANT UPDATE ON
    users, workspaces, datasets, dataset_object_cleanup_intents,
    run_artifact_upload_intents, artifacts, download_object_cleanup_intents,
    analytics_events, feedback_reports, skill_sources
    TO skillhub_purge;

GRANT INSERT ON object_collection_queue, audit_events TO skillhub_purge;

GRANT DELETE ON
    datasets, dataset_object_cleanup_intents, test_cases, run_artifact_upload_intents,
    artifacts, download_object_cleanup_intents, download_records, download_artifacts,
    creation_sessions, skill_versions, skills, skill_sources, user_identities, sessions,
    object_collection_queue, object_reconcile_sightings, audit_events, feedback_reports
    TO skillhub_purge;

