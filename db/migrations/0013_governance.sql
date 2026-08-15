-- 0013_governance: account deletion grace period, audit events, manual takedown,
-- and source availability marking (CORE-007, CORE-008, INGEST-010).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

-- CORE-007 ------------------------------------------------------------------
-- 0002 reserved users.deleted_at for account deletion, but every read filters on
-- `deleted_at IS NULL` and login reuses that same filter to decide "this identity
-- has no account yet". Setting it when the user *asks* to be deleted would log
-- them out of the grace period they are supposed to be able to cancel from, and a
-- second login would silently create a duplicate account. So the grace period gets
-- its own column and the two states stay distinguishable:
--
--   deletion_requested_at  grace started, account still fully usable, cancellable
--   deleted_at             the purge ran; private content is gone and what had to
--                          be retained no longer names this person (PDM-006 6.1)
ALTER TABLE users ADD COLUMN deletion_requested_at timestamptz;

-- Worklist index for the purge: the pending set is a small tail of the table.
CREATE INDEX users_deletion_requested_at_idx ON users (deletion_requested_at)
    WHERE deletion_requested_at IS NOT NULL AND deleted_at IS NULL;

-- The purge has to hard-delete rows of tables 0005 froze (skill_versions of
-- content nobody else references). Retention deletion was always outside the
-- immutability rule — trace_events retention is a DROP PARTITION, which the
-- trigger never saw — this makes the same exemption reachable for row deletes
-- without weakening the rule for application code: UPDATE stays blocked
-- unconditionally, and DELETE only opens inside a transaction that has set the
-- flag with SET LOCAL, which nothing but the purge does.
CREATE OR REPLACE FUNCTION enforce_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    old_row jsonb;
    new_row jsonb;
    mutable_col text;
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF coalesce(current_setting('skillhub.purge', true), '') = 'on' THEN
            RETURN OLD; -- retention purge (PDM-006 6.1), never application code
        END IF;
        RAISE EXCEPTION 'row in %.% is immutable and cannot be deleted (ADR-003)',
            TG_TABLE_SCHEMA, TG_TABLE_NAME
            USING ERRCODE = 'restrict_violation';
    END IF;

    old_row := to_jsonb(OLD);
    new_row := to_jsonb(NEW);

    IF TG_NARGS > 0 THEN
        FOREACH mutable_col IN ARRAY TG_ARGV LOOP
            old_row := old_row - mutable_col;
            new_row := new_row - mutable_col;
        END LOOP;
    END IF;

    IF old_row IS DISTINCT FROM new_row THEN
        RAISE EXCEPTION 'row in %.% is immutable and cannot be updated (ADR-003)',
            TG_TABLE_SCHEMA, TG_TABLE_NAME
            USING ERRCODE = 'restrict_violation';
    END IF;

    RETURN NEW;
END;
$$;

-- CORE-008 ------------------------------------------------------------------
-- Append-only record of the operations NFR-001 names: import, execution,
-- permission confirmation, download, deletion. slog is operational logging and
-- gets rotated; this is the auditable trail.
CREATE TABLE audit_events (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- FK, not free text: account purge de-identifies the users row in place
    -- (PDM-006 6.1 "僅保留去識別化 actor ID"), so the reference stays valid and
    -- the event keeps pointing at a user who can no longer be identified —
    -- without the audit row itself ever being rewritten.
    -- NULL = anonymous or platform-initiated.
    actor_user_id uuid REFERENCES users (id),
    workspace_id  uuid REFERENCES workspaces (id),
    action        text NOT NULL, -- see internal/audit for the vocabulary
    resource_type text NOT NULL,
    resource_id   uuid,
    -- Identifiers and outcome only. 400 day retention (PDM-006 6) is affordable
    -- exactly because this column never carries user content, and iron rule 11
    -- forbids secrets in it.
    metadata      jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Retention sweep (400 days) scans by age; the actor index serves "what did this
-- account do" during an incident.
CREATE INDEX audit_events_created_at_idx ON audit_events (created_at);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_user_id, created_at DESC);

-- Insert-only. Same trigger as every other append-only log, so the 400 day
-- retention delete goes through the purge flag above like the rest.
CREATE TRIGGER audit_events_immutable
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

-- INGEST-010 ----------------------------------------------------------------
-- Manual takedown is *not* deletion: PDM-006 keeps skills, sources and versions
-- permanently and lists "僅人工下架（標記不可見，內容保留）" as the only removal
-- path. The row stays, the search projection row does not, and fork stops
-- resolving it — existing forks and historical lineage are untouched.
ALTER TABLE skills
    ADD COLUMN takedown_at     timestamptz,
    ADD COLUMN takedown_reason text,
    -- A takedown with no stated reason is not reviewable later.
    ADD CONSTRAINT skills_takedown_needs_reason
        CHECK (takedown_at IS NULL OR btrim(coalesce(takedown_reason, '')) <> '');

-- Upstream repos get deleted or rewritten (threat model TM-SUP-02, and the
-- `data` category candidates are 1-5 star personal repos). The probe records
-- both facts separately: when we last looked, and since when it has been gone.
ALTER TABLE skill_sources
    ADD COLUMN last_checked_at   timestamptz,
    ADD COLUMN unavailable_since timestamptz;
