ALTER TABLE feedback_reports ALTER COLUMN message DROP NOT NULL;

ALTER TABLE feedback_reports
    ADD CONSTRAINT feedback_reports_message_kept_while_attributed
    CHECK (message IS NOT NULL OR (workspace_id IS NULL AND user_id IS NULL));
