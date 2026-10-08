ALTER TABLE creation_session_events DROP COLUMN snapshot, ADD COLUMN state text;
ALTER TABLE creation_session_events
    ADD CONSTRAINT creation_session_events_state_recorded CHECK (state IS NOT NULL AND btrim(state) <> '') NOT VALID;
