ALTER TABLE creation_receipts
    ADD CONSTRAINT creation_receipts_status_check
    CHECK (status IN ('queued', 'running', 'unknown', 'finished', 'failed', 'cancelled'));

ALTER TABLE creation_sessions
    ADD CONSTRAINT creation_sessions_state_check
    CHECK (state IN ('queued', 'working', 'waiting_input', 'waiting_confirmation', 'draft_ready', 'candidate_ready',
                     'saved', 'cancelled', 'failed', 'needs_reupload'));

ALTER TABLE creation_session_events
    DROP CONSTRAINT creation_session_events_state_recorded,
    ADD CONSTRAINT creation_session_events_state_check
    CHECK (state IN ('queued', 'working', 'waiting_input', 'waiting_confirmation', 'draft_ready', 'candidate_ready',
                     'saved', 'cancelled', 'failed', 'needs_reupload'));

COMMENT ON COLUMN creation_session_events.state IS
    'The session''s state after this revision was applied: queued, working, waiting_input, waiting_confirmation, '
    'draft_ready, candidate_ready, saved, cancelled, failed or needs_reupload, the same vocabulary as '
    'creation_sessions.state.';

ALTER TABLE trace_events
    ADD CONSTRAINT trace_events_status_check
    CHECK (status IS NULL OR status IN ('ok', 'error', 'skipped', 'cancelled', 'timed_out'));
