ALTER TABLE creation_session_events
    DROP CONSTRAINT creation_session_events_state_check,
    ADD CONSTRAINT creation_session_events_state_check
    CHECK (state IS NOT NULL AND state IN ('queued', 'working', 'waiting_input', 'waiting_confirmation', 'draft_ready',
                                           'candidate_ready', 'saved', 'cancelled', 'failed', 'needs_reupload')) NOT VALID;
