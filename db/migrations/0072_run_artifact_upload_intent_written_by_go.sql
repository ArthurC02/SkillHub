DROP TRIGGER run_attempt_artifact_upload_intent ON run_attempts;
DROP FUNCTION remember_run_artifact_upload_intent();

ALTER TABLE dataset_object_cleanup_intents ALTER COLUMN not_before DROP DEFAULT;
ALTER TABLE download_object_cleanup_intents ALTER COLUMN not_before DROP DEFAULT;
