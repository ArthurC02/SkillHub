ALTER TABLE artifacts
    DROP CONSTRAINT artifacts_run_id_fkey,
    ADD CONSTRAINT artifacts_run_id_fkey FOREIGN KEY (run_id, workspace_id) REFERENCES runs(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT artifacts_workspace_id_fkey,
    ADD CONSTRAINT artifacts_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE audit_events
    DROP CONSTRAINT audit_events_actor_agent_id_fkey,
    ADD CONSTRAINT audit_events_actor_agent_id_fkey FOREIGN KEY (actor_agent_id) REFERENCES platform_agents(id) ON DELETE NO ACTION,
    DROP CONSTRAINT audit_events_actor_user_id_fkey,
    ADD CONSTRAINT audit_events_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE NO ACTION,
    DROP CONSTRAINT audit_events_workspace_id_fkey,
    ADD CONSTRAINT audit_events_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE bundle_members
    DROP CONSTRAINT bundle_members_skill_version_id_fkey,
    ADD CONSTRAINT bundle_members_skill_version_id_fkey FOREIGN KEY (skill_version_id) REFERENCES skill_versions(id) ON DELETE NO ACTION;

ALTER TABLE bundles
    DROP CONSTRAINT bundles_workspace_id_fkey,
    ADD CONSTRAINT bundles_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE bundle_versions
    DROP CONSTRAINT bundle_versions_created_by_fkey,
    ADD CONSTRAINT bundle_versions_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE NO ACTION;

ALTER TABLE cost_events
    DROP CONSTRAINT cost_events_user_id_fkey,
    ADD CONSTRAINT cost_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE NO ACTION,
    DROP CONSTRAINT cost_events_workspace_id_fkey,
    ADD CONSTRAINT cost_events_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE cost_session_summaries
    DROP CONSTRAINT cost_session_summaries_user_id_fkey,
    ADD CONSTRAINT cost_session_summaries_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE NO ACTION;

ALTER TABLE creation_sessions
    DROP CONSTRAINT creation_sessions_workspace_id_fkey,
    ADD CONSTRAINT creation_sessions_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE credit_accounts
    DROP CONSTRAINT credit_accounts_user_id_fkey,
    ADD CONSTRAINT credit_accounts_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE NO ACTION;

ALTER TABLE credit_entries
    DROP CONSTRAINT credit_entries_cost_event_id_fkey,
    ADD CONSTRAINT credit_entries_cost_event_id_fkey FOREIGN KEY (cost_event_id) REFERENCES cost_events(id) ON DELETE NO ACTION,
    DROP CONSTRAINT credit_entries_user_id_fkey,
    ADD CONSTRAINT credit_entries_user_id_fkey FOREIGN KEY (user_id) REFERENCES credit_accounts(user_id) ON DELETE NO ACTION;

ALTER TABLE dataset_object_cleanup_intents
    DROP CONSTRAINT dataset_object_cleanup_intents_workspace_id_fkey,
    ADD CONSTRAINT dataset_object_cleanup_intents_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE datasets
    DROP CONSTRAINT datasets_test_case_id_fkey,
    ADD CONSTRAINT datasets_test_case_id_fkey FOREIGN KEY (test_case_id, workspace_id) REFERENCES test_cases(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT datasets_workspace_id_fkey,
    ADD CONSTRAINT datasets_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE download_artifact_members
    DROP CONSTRAINT download_artifact_members_artifact_fkey,
    ADD CONSTRAINT download_artifact_members_artifact_fkey FOREIGN KEY (artifact_id, workspace_id) REFERENCES download_artifacts(artifact_id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT download_artifact_members_skill_version_id_fkey,
    ADD CONSTRAINT download_artifact_members_skill_version_id_fkey FOREIGN KEY (skill_version_id) REFERENCES skill_versions(id) ON DELETE NO ACTION;

ALTER TABLE download_artifacts
    DROP CONSTRAINT download_artifacts_artifact_fkey,
    ADD CONSTRAINT download_artifacts_artifact_fkey FOREIGN KEY (artifact_id, kind, workspace_id) REFERENCES artifacts(id, kind, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT download_artifacts_skill_version_id_fkey,
    ADD CONSTRAINT download_artifacts_skill_version_id_fkey FOREIGN KEY (skill_version_id) REFERENCES skill_versions(id) ON DELETE NO ACTION,
    DROP CONSTRAINT download_artifacts_workspace_id_fkey,
    ADD CONSTRAINT download_artifacts_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE download_object_cleanup_intents
    DROP CONSTRAINT download_object_cleanup_intents_workspace_id_fkey,
    ADD CONSTRAINT download_object_cleanup_intents_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE download_records
    DROP CONSTRAINT download_records_actor_user_id_fkey,
    ADD CONSTRAINT download_records_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE NO ACTION,
    DROP CONSTRAINT download_records_artifact_fkey,
    ADD CONSTRAINT download_records_artifact_fkey FOREIGN KEY (artifact_id, workspace_id) REFERENCES download_artifacts(artifact_id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT download_records_workspace_id_fkey,
    ADD CONSTRAINT download_records_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE evaluation_grader_versions
    DROP CONSTRAINT evaluation_grader_versions_created_by_fkey,
    ADD CONSTRAINT evaluation_grader_versions_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE NO ACTION;

ALTER TABLE evaluation_model_usage
    DROP CONSTRAINT evaluation_model_usage_evaluation_workspace_fk,
    ADD CONSTRAINT evaluation_model_usage_evaluation_workspace_fk FOREIGN KEY (evaluation_id, workspace_id) REFERENCES evaluations(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT evaluation_model_usage_workspace_id_fkey,
    ADD CONSTRAINT evaluation_model_usage_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE evaluations
    DROP CONSTRAINT evaluations_run_id_fkey,
    ADD CONSTRAINT evaluations_run_id_fkey FOREIGN KEY (run_id, workspace_id) REFERENCES runs(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT evaluations_workspace_id_fkey,
    ADD CONSTRAINT evaluations_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE evaluation_suggestion_applications
    DROP CONSTRAINT evaluation_suggestion_applicati_suggestion_id_workspace_id_fkey,
    ADD CONSTRAINT evaluation_suggestion_applicati_suggestion_id_workspace_id_fkey FOREIGN KEY (suggestion_id, workspace_id) REFERENCES evaluation_suggestions(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT evaluation_suggestion_applications_workspace_id_fkey,
    ADD CONSTRAINT evaluation_suggestion_applications_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE evaluation_suggestions
    DROP CONSTRAINT evaluation_suggestions_applied_skill_version_id_fkey,
    ADD CONSTRAINT evaluation_suggestions_applied_skill_version_id_fkey FOREIGN KEY (applied_skill_version_id) REFERENCES skill_versions(id) ON DELETE NO ACTION,
    DROP CONSTRAINT evaluation_suggestions_evaluation_id_fkey,
    ADD CONSTRAINT evaluation_suggestions_evaluation_id_fkey FOREIGN KEY (evaluation_id, workspace_id) REFERENCES evaluations(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT evaluation_suggestions_workspace_id_fkey,
    ADD CONSTRAINT evaluation_suggestions_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE exposure_reviews
    DROP CONSTRAINT exposure_reviews_reviewer_user_id_fkey,
    ADD CONSTRAINT exposure_reviews_reviewer_user_id_fkey FOREIGN KEY (reviewer_user_id) REFERENCES users(id) ON DELETE NO ACTION;

ALTER TABLE outbox_events
    DROP CONSTRAINT outbox_events_workspace_id_fkey,
    ADD CONSTRAINT outbox_events_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE platform_agent_brake
    DROP CONSTRAINT platform_agent_brake_engaged_by_fkey,
    ADD CONSTRAINT platform_agent_brake_engaged_by_fkey FOREIGN KEY (engaged_by) REFERENCES users(id) ON DELETE NO ACTION;

ALTER TABLE platform_agent_finding_events
    DROP CONSTRAINT platform_agent_finding_events_finding_id_fkey,
    ADD CONSTRAINT platform_agent_finding_events_finding_id_fkey FOREIGN KEY (finding_id) REFERENCES platform_agent_findings(id) ON DELETE NO ACTION,
    DROP CONSTRAINT platform_agent_finding_events_operator_id_fkey,
    ADD CONSTRAINT platform_agent_finding_events_operator_id_fkey FOREIGN KEY (operator_id) REFERENCES users(id) ON DELETE NO ACTION,
    DROP CONSTRAINT platform_agent_finding_events_run_id_fkey,
    ADD CONSTRAINT platform_agent_finding_events_run_id_fkey FOREIGN KEY (run_id) REFERENCES platform_agent_runs(id) ON DELETE NO ACTION;

ALTER TABLE platform_agent_findings
    DROP CONSTRAINT platform_agent_findings_agent_id_fkey,
    ADD CONSTRAINT platform_agent_findings_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES platform_agents(id) ON DELETE NO ACTION,
    DROP CONSTRAINT platform_agent_findings_assignee_id_fkey,
    ADD CONSTRAINT platform_agent_findings_assignee_id_fkey FOREIGN KEY (assignee_id) REFERENCES users(id) ON DELETE NO ACTION;

ALTER TABLE platform_agent_proposals
    DROP CONSTRAINT platform_agent_proposals_agent_id_fkey,
    ADD CONSTRAINT platform_agent_proposals_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES platform_agents(id) ON DELETE NO ACTION,
    DROP CONSTRAINT platform_agent_proposals_decided_by_fkey,
    ADD CONSTRAINT platform_agent_proposals_decided_by_fkey FOREIGN KEY (decided_by) REFERENCES users(id) ON DELETE NO ACTION,
    DROP CONSTRAINT platform_agent_proposals_run_id_fkey,
    ADD CONSTRAINT platform_agent_proposals_run_id_fkey FOREIGN KEY (run_id) REFERENCES platform_agent_runs(id) ON DELETE NO ACTION;

ALTER TABLE platform_agent_runs
    DROP CONSTRAINT platform_agent_runs_agent_id_fkey,
    ADD CONSTRAINT platform_agent_runs_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES platform_agents(id) ON DELETE NO ACTION;

ALTER TABLE platform_agents
    DROP CONSTRAINT platform_agents_owner_id_fkey,
    ADD CONSTRAINT platform_agents_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE NO ACTION;

ALTER TABLE platform_agent_steps
    DROP CONSTRAINT platform_agent_steps_run_id_fkey,
    ADD CONSTRAINT platform_agent_steps_run_id_fkey FOREIGN KEY (run_id) REFERENCES platform_agent_runs(id) ON DELETE NO ACTION;

ALTER TABLE publication_releases
    DROP CONSTRAINT publication_releases_released_by_fkey,
    ADD CONSTRAINT publication_releases_released_by_fkey FOREIGN KEY (released_by) REFERENCES users(id) ON DELETE NO ACTION;

ALTER TABLE publications
    DROP CONSTRAINT publications_publisher_id_fkey,
    ADD CONSTRAINT publications_publisher_id_fkey FOREIGN KEY (publisher_id) REFERENCES publishers(id) ON DELETE NO ACTION;

ALTER TABLE publishers
    DROP CONSTRAINT publishers_workspace_id_fkey,
    ADD CONSTRAINT publishers_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE run_artifact_upload_intents
    DROP CONSTRAINT run_artifact_upload_intents_workspace_id_fkey,
    ADD CONSTRAINT run_artifact_upload_intents_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE run_attempts
    DROP CONSTRAINT run_attempts_run_id_fkey,
    ADD CONSTRAINT run_attempts_run_id_fkey FOREIGN KEY (run_id, workspace_id) REFERENCES runs(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT run_attempts_workspace_id_fkey,
    ADD CONSTRAINT run_attempts_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE run_permission_confirmations
    DROP CONSTRAINT run_permission_confirmations_confirmed_by_fkey,
    ADD CONSTRAINT run_permission_confirmations_confirmed_by_fkey FOREIGN KEY (confirmed_by) REFERENCES users(id) ON DELETE NO ACTION,
    DROP CONSTRAINT run_permission_confirmations_workspace_id_fkey,
    ADD CONSTRAINT run_permission_confirmations_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE runs
    DROP CONSTRAINT runs_skill_version_id_fkey,
    ADD CONSTRAINT runs_skill_version_id_fkey FOREIGN KEY (skill_version_id, workspace_id) REFERENCES skill_versions(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT runs_test_case_snapshot_id_fkey,
    ADD CONSTRAINT runs_test_case_snapshot_id_fkey FOREIGN KEY (test_case_snapshot_id, workspace_id) REFERENCES test_case_snapshots(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT runs_workspace_id_fkey,
    ADD CONSTRAINT runs_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE run_snapshots
    DROP CONSTRAINT run_snapshots_run_id_fkey,
    ADD CONSTRAINT run_snapshots_run_id_fkey FOREIGN KEY (run_id, workspace_id) REFERENCES runs(id, workspace_id) ON DELETE NO ACTION;

ALTER TABLE run_status_transitions
    DROP CONSTRAINT run_status_transitions_run_attempt_id_fkey,
    ADD CONSTRAINT run_status_transitions_run_attempt_id_fkey FOREIGN KEY (run_attempt_id) REFERENCES run_attempts(id) ON DELETE NO ACTION,
    DROP CONSTRAINT run_status_transitions_run_id_fkey,
    ADD CONSTRAINT run_status_transitions_run_id_fkey FOREIGN KEY (run_id, workspace_id) REFERENCES runs(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT run_status_transitions_workspace_id_fkey,
    ADD CONSTRAINT run_status_transitions_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE search_documents
    DROP CONSTRAINT search_documents_workspace_id_fkey,
    ADD CONSTRAINT search_documents_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE skill_runtime_compatibility
    DROP CONSTRAINT skill_runtime_compatibility_source_run_id_fkey,
    ADD CONSTRAINT skill_runtime_compatibility_source_run_id_fkey FOREIGN KEY (source_run_id) REFERENCES runs(id) ON DELETE NO ACTION;

ALTER TABLE skills
    DROP CONSTRAINT skills_forked_from_skill_id_fkey,
    ADD CONSTRAINT skills_forked_from_skill_id_fkey FOREIGN KEY (forked_from_skill_id) REFERENCES skills(id) ON DELETE NO ACTION,
    DROP CONSTRAINT skills_forked_from_version_id_fkey,
    ADD CONSTRAINT skills_forked_from_version_id_fkey FOREIGN KEY (forked_from_version_id) REFERENCES skill_versions(id) ON DELETE NO ACTION,
    DROP CONSTRAINT skills_workspace_id_fkey,
    ADD CONSTRAINT skills_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE skill_sources
    DROP CONSTRAINT skill_sources_workspace_id_fkey,
    ADD CONSTRAINT skill_sources_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE skill_versions
    DROP CONSTRAINT skill_versions_skill_id_fkey,
    ADD CONSTRAINT skill_versions_skill_id_fkey FOREIGN KEY (skill_id, workspace_id) REFERENCES skills(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT skill_versions_source_id_fkey,
    ADD CONSTRAINT skill_versions_source_id_fkey FOREIGN KEY (source_id) REFERENCES skill_sources(id) ON DELETE NO ACTION,
    DROP CONSTRAINT skill_versions_workspace_id_fkey,
    ADD CONSTRAINT skill_versions_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE test_cases
    DROP CONSTRAINT test_cases_skill_id_fkey,
    ADD CONSTRAINT test_cases_skill_id_fkey FOREIGN KEY (skill_id, workspace_id) REFERENCES skills(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT test_cases_workspace_id_fkey,
    ADD CONSTRAINT test_cases_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE test_case_snapshots
    DROP CONSTRAINT test_case_snapshots_test_case_id_fkey,
    ADD CONSTRAINT test_case_snapshots_test_case_id_fkey FOREIGN KEY (test_case_id, workspace_id) REFERENCES test_cases(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT test_case_snapshots_workspace_id_fkey,
    ADD CONSTRAINT test_case_snapshots_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE trace_events
    DROP CONSTRAINT trace_events_run_id_fkey,
    ADD CONSTRAINT trace_events_run_id_fkey FOREIGN KEY (run_id, workspace_id) REFERENCES runs(id, workspace_id) ON DELETE NO ACTION,
    DROP CONSTRAINT trace_events_workspace_id_fkey,
    ADD CONSTRAINT trace_events_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE NO ACTION;

ALTER TABLE user_identities
    DROP CONSTRAINT user_identities_user_id_fkey,
    ADD CONSTRAINT user_identities_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE NO ACTION;

ALTER TABLE workspaces
    DROP CONSTRAINT workspaces_owner_user_id_fkey,
    ADD CONSTRAINT workspaces_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE NO ACTION;

DROP TRIGGER account_purge_insert_fence ON skill_sources;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON skill_sources
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON skills;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON skills
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON skill_versions;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON skill_versions
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON test_cases;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON test_cases
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON datasets;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON datasets
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON test_case_snapshots;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON test_case_snapshots
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON runs;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON runs
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON run_status_transitions;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON run_status_transitions
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON trace_events;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON trace_events
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON evaluations;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON evaluations
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON artifacts;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON artifacts
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON run_attempts;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON run_attempts
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON outbox_events;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON outbox_events
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON run_permission_confirmations;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON run_permission_confirmations
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON evaluation_suggestions;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON evaluation_suggestions
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON download_artifacts;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON download_artifacts
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON download_records;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON download_records
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON analytics_events;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON analytics_events
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON feedback_reports;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON feedback_reports
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON evaluation_model_usage;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON evaluation_model_usage
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

DROP TRIGGER account_purge_insert_fence ON dataset_object_cleanup_intents;
CREATE TRIGGER account_purge_insert_fence
BEFORE INSERT ON dataset_object_cleanup_intents
FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();
