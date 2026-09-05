-- GEN-011: a creation session or command id must not be an existence oracle
-- across workspaces. Both ids are client-supplied (idempotency handles), so a
-- bare global primary key made "409 for a foreign id, 200 for a free one" a way
-- to learn that somebody else's session exists. Scope the keys instead: the
-- same id in two workspaces is two rows, and the workspace-scoped reads that
-- already exist keep them apart.
ALTER TABLE creation_sessions DROP CONSTRAINT creation_sessions_pkey;
ALTER TABLE creation_sessions ADD PRIMARY KEY (id, workspace_id);
ALTER TABLE creation_receipts DROP CONSTRAINT creation_receipts_pkey;
ALTER TABLE creation_receipts ADD PRIMARY KEY (id, session_id, workspace_id);
-- creation_session_events carried the same global shape through (session_id, revision);
-- a foreign session id would have collided here on revision 1 and turned the closed
-- oracle into a 503 that still says "this id exists". Scope it the same way.
ALTER TABLE creation_session_events DROP CONSTRAINT creation_session_events_pkey;
ALTER TABLE creation_session_events ADD PRIMARY KEY (session_id, workspace_id, revision);
