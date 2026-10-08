ALTER TABLE skills ADD CONSTRAINT skills_id_workspace_key UNIQUE (id, workspace_id);
ALTER TABLE test_cases ADD CONSTRAINT test_cases_id_workspace_key UNIQUE (id, workspace_id);
ALTER TABLE test_case_snapshots ADD CONSTRAINT test_case_snapshots_id_workspace_key UNIQUE (id, workspace_id);
ALTER TABLE runs ADD CONSTRAINT runs_id_workspace_key UNIQUE (id, workspace_id);
ALTER TABLE publication_releases ADD CONSTRAINT publication_releases_id_publication_key UNIQUE (id, publication_id);

ALTER TABLE skill_versions DROP CONSTRAINT skill_versions_skill_id_fkey,
    ADD CONSTRAINT skill_versions_skill_id_fkey
    FOREIGN KEY (skill_id, workspace_id) REFERENCES skills (id, workspace_id);
ALTER TABLE test_cases DROP CONSTRAINT test_cases_skill_id_fkey,
    ADD CONSTRAINT test_cases_skill_id_fkey
    FOREIGN KEY (skill_id, workspace_id) REFERENCES skills (id, workspace_id);
ALTER TABLE test_case_snapshots DROP CONSTRAINT test_case_snapshots_test_case_id_fkey,
    ADD CONSTRAINT test_case_snapshots_test_case_id_fkey
    FOREIGN KEY (test_case_id, workspace_id) REFERENCES test_cases (id, workspace_id);
ALTER TABLE datasets DROP CONSTRAINT datasets_test_case_id_fkey,
    ADD CONSTRAINT datasets_test_case_id_fkey
    FOREIGN KEY (test_case_id, workspace_id) REFERENCES test_cases (id, workspace_id);
ALTER TABLE runs DROP CONSTRAINT runs_skill_version_id_fkey,
    ADD CONSTRAINT runs_skill_version_id_fkey
    FOREIGN KEY (skill_version_id, workspace_id) REFERENCES skill_versions (id, workspace_id);
ALTER TABLE runs DROP CONSTRAINT runs_test_case_snapshot_id_fkey,
    ADD CONSTRAINT runs_test_case_snapshot_id_fkey
    FOREIGN KEY (test_case_snapshot_id, workspace_id) REFERENCES test_case_snapshots (id, workspace_id);
ALTER TABLE run_attempts DROP CONSTRAINT run_attempts_run_id_fkey,
    ADD CONSTRAINT run_attempts_run_id_fkey
    FOREIGN KEY (run_id, workspace_id) REFERENCES runs (id, workspace_id);
ALTER TABLE run_status_transitions DROP CONSTRAINT run_status_transitions_run_id_fkey,
    ADD CONSTRAINT run_status_transitions_run_id_fkey
    FOREIGN KEY (run_id, workspace_id) REFERENCES runs (id, workspace_id);
ALTER TABLE trace_events DROP CONSTRAINT trace_events_run_id_fkey,
    ADD CONSTRAINT trace_events_run_id_fkey
    FOREIGN KEY (run_id, workspace_id) REFERENCES runs (id, workspace_id);
ALTER TABLE evaluations DROP CONSTRAINT evaluations_run_id_fkey,
    ADD CONSTRAINT evaluations_run_id_fkey
    FOREIGN KEY (run_id, workspace_id) REFERENCES runs (id, workspace_id);
ALTER TABLE artifacts DROP CONSTRAINT artifacts_run_id_fkey,
    ADD CONSTRAINT artifacts_run_id_fkey
    FOREIGN KEY (run_id, workspace_id) REFERENCES runs (id, workspace_id);
ALTER TABLE evaluation_suggestions DROP CONSTRAINT evaluation_suggestions_evaluation_id_fkey,
    ADD CONSTRAINT evaluation_suggestions_evaluation_id_fkey
    FOREIGN KEY (evaluation_id, workspace_id) REFERENCES evaluations (id, workspace_id);
ALTER TABLE exposure_reviews DROP CONSTRAINT exposure_reviews_release_id_fkey,
    ADD CONSTRAINT exposure_reviews_release_id_fkey
    FOREIGN KEY (release_id, publication_id) REFERENCES publication_releases (id, publication_id) ON DELETE CASCADE;
