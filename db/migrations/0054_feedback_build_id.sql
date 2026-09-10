ALTER TABLE feedback_reports
    ADD COLUMN build_id text CHECK (build_id IS NULL OR length(build_id) <= 64);
