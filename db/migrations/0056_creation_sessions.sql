-- ADR-067: Go owns durable creation facts. Original diagrams are never stored.
CREATE TABLE creation_sessions (
 id uuid PRIMARY KEY,
 workspace_id uuid NOT NULL REFERENCES workspaces(id),
 state text NOT NULL,
 revision bigint NOT NULL CHECK (revision >= 1),
 snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 UNIQUE(id, workspace_id)
);
CREATE INDEX creation_sessions_workspace ON creation_sessions(workspace_id, updated_at DESC);
CREATE INDEX creation_sessions_expiry ON creation_sessions(expires_at);
CREATE TABLE creation_session_events (
 session_id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 revision bigint NOT NULL,
 event_type text NOT NULL,
 snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(session_id, revision),
 FOREIGN KEY(session_id,workspace_id) REFERENCES creation_sessions(id,workspace_id) ON DELETE CASCADE
);
CREATE TABLE creation_receipts (
 id uuid PRIMARY KEY,
 session_id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 kind text NOT NULL CHECK (kind IN ('command','attempt')),
 status text NOT NULL,
 expected_revision bigint NOT NULL,
 request_hash text NOT NULL,
 result jsonb NOT NULL DEFAULT '{}',
 usage jsonb NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now(),
 finished_at timestamptz,
 FOREIGN KEY(session_id,workspace_id) REFERENCES creation_sessions(id,workspace_id) ON DELETE CASCADE
);
CREATE INDEX creation_receipts_session ON creation_receipts(session_id, workspace_id);
CREATE FUNCTION forbid_creation_event_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'creation event is immutable';
END;
$$;
CREATE TRIGGER creation_event_immutable BEFORE UPDATE ON creation_session_events
 FOR EACH ROW EXECUTE FUNCTION forbid_creation_event_update();
CREATE TRIGGER account_purge_insert_fence BEFORE INSERT ON creation_sessions
 FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();
CREATE TRIGGER account_purge_update_fence BEFORE UPDATE ON creation_sessions
 FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();
CREATE TRIGGER account_purge_insert_fence BEFORE INSERT ON creation_session_events
 FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();
CREATE TRIGGER account_purge_insert_fence BEFORE INSERT ON creation_receipts
 FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();
CREATE TRIGGER account_purge_update_fence BEFORE UPDATE ON creation_receipts
 FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();
