DROP INDEX run_attempts_run_id_idx;
DROP INDEX cost_statistics_kind_window_end_idx;
DROP INDEX evaluation_model_usage_evaluation_id_idx;
DROP INDEX users_deletion_requested_at_idx;
DROP INDEX runs_cleanup_backlog_idx;
DROP INDEX search_documents_pending_enrichment_idx;

DROP INDEX trace_events_workspace_id_idx;
DROP INDEX analytics_events_session_idx;
DROP INDEX audit_events_actor_idx;
DROP INDEX cost_events_user_created_at_idx;
DROP INDEX feedback_reports_kind_idx;
DROP INDEX artifacts_expires_at_idx;
DROP INDEX artifacts_unpurged_expiry_idx;
DROP INDEX datasets_expires_at_idx;
DROP INDEX skill_sources_content_hash_idx;
DROP INDEX evaluation_suggestions_workspace_id_idx;
DROP INDEX skill_versions_workspace_id_idx;
DROP INDEX run_attempts_workspace_id_idx;
DROP INDEX test_case_snapshots_workspace_id_idx;

ALTER TABLE creation_session_events DROP CONSTRAINT creation_session_events_session_id_workspace_id_fkey;
ALTER TABLE creation_receipts DROP CONSTRAINT creation_receipts_session_id_workspace_id_fkey;
ALTER TABLE creation_sessions DROP CONSTRAINT creation_sessions_id_workspace_id_key;
ALTER TABLE creation_session_events ADD CONSTRAINT creation_session_events_session_id_workspace_id_fkey
    FOREIGN KEY (session_id, workspace_id) REFERENCES creation_sessions (id, workspace_id) ON DELETE CASCADE;
ALTER TABLE creation_receipts ADD CONSTRAINT creation_receipts_session_id_workspace_id_fkey
    FOREIGN KEY (session_id, workspace_id) REFERENCES creation_sessions (id, workspace_id) ON DELETE CASCADE;

CREATE INDEX skill_versions_source_id_idx ON skill_versions (source_id, version_number DESC);
CREATE INDEX skills_forked_from_skill_id_idx ON skills (forked_from_skill_id)
    WHERE forked_from_skill_id IS NOT NULL;
CREATE INDEX run_permission_confirmations_skill_version_id_idx ON run_permission_confirmations (skill_version_id);
CREATE INDEX run_permission_confirmations_test_case_id_idx ON run_permission_confirmations (test_case_id);
CREATE INDEX publication_releases_skill_version_id_idx ON publication_releases (skill_version_id)
    WHERE skill_version_id IS NOT NULL;
CREATE INDEX publication_releases_bundle_version_id_idx ON publication_releases (bundle_version_id)
    WHERE bundle_version_id IS NOT NULL;
CREATE INDEX exposure_reviews_release_id_idx ON exposure_reviews (release_id);
CREATE INDEX evaluation_suggestions_applied_skill_version_id_idx ON evaluation_suggestions (applied_skill_version_id)
    WHERE applied_skill_version_id IS NOT NULL;
CREATE INDEX cost_events_ref_id_idx ON cost_events (ref_id, created_at) WHERE ref_id IS NOT NULL;
CREATE INDEX evaluations_awaiting_idx ON evaluations (status, created_at) WHERE superseded_at IS NULL;
