ALTER TABLE creation_sessions DROP CONSTRAINT creation_sessions_pkey;
ALTER TABLE creation_sessions ADD PRIMARY KEY (id, workspace_id);
ALTER TABLE creation_receipts DROP CONSTRAINT creation_receipts_pkey;
ALTER TABLE creation_receipts ADD PRIMARY KEY (id, session_id, workspace_id);
ALTER TABLE creation_session_events DROP CONSTRAINT creation_session_events_pkey;
ALTER TABLE creation_session_events ADD PRIMARY KEY (session_id, workspace_id, revision);
