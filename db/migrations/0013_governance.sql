ALTER TABLE users ADD COLUMN deletion_requested_at timestamptz;

CREATE INDEX users_deletion_requested_at_idx ON users (deletion_requested_at)
    WHERE deletion_requested_at IS NOT NULL AND deleted_at IS NULL;

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

CREATE TABLE audit_events (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id uuid REFERENCES users (id),
    workspace_id  uuid REFERENCES workspaces (id),
    action        text NOT NULL, -- see internal/audit for the vocabulary
    resource_type text NOT NULL,
    resource_id   uuid,
    metadata      jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_events_created_at_idx ON audit_events (created_at);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_user_id, created_at DESC);

CREATE TRIGGER audit_events_immutable
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

ALTER TABLE skills
    ADD COLUMN takedown_at     timestamptz,
    ADD COLUMN takedown_reason text,
    ADD CONSTRAINT skills_takedown_needs_reason
        CHECK (takedown_at IS NULL OR btrim(coalesce(takedown_reason, '')) <> '');

ALTER TABLE skill_sources
    ADD COLUMN last_checked_at   timestamptz,
    ADD COLUMN unavailable_since timestamptz;
