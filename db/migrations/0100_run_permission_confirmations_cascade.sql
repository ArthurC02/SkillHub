ALTER TABLE run_permission_confirmations
    DROP CONSTRAINT run_permission_confirmations_test_case_id_fkey,
    ADD CONSTRAINT run_permission_confirmations_test_case_id_fkey
        FOREIGN KEY (test_case_id) REFERENCES test_cases (id) ON DELETE CASCADE,
    DROP CONSTRAINT run_permission_confirmations_skill_version_id_fkey,
    ADD CONSTRAINT run_permission_confirmations_skill_version_id_fkey
        FOREIGN KEY (skill_version_id) REFERENCES skill_versions (id) ON DELETE CASCADE;
