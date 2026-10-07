CREATE TABLE run_snapshots (
    run_id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    runtime_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    policy_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT run_snapshots_run_id_fkey FOREIGN KEY (run_id, workspace_id) REFERENCES runs (id, workspace_id)
);

INSERT INTO run_snapshots (run_id, workspace_id, runtime_snapshot, policy_snapshot)
SELECT id, workspace_id, runtime_snapshot, policy_snapshot FROM runs;

ALTER TABLE runs DROP COLUMN runtime_snapshot, DROP COLUMN policy_snapshot;

CREATE FUNCTION enforce_run_snapshot_open() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM runs WHERE id = NEW.run_id AND finished_at IS NOT NULL) THEN
        RAISE EXCEPTION 'the runtime snapshot of finished run % is immutable', NEW.run_id
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER run_snapshots_immutable BEFORE DELETE OR UPDATE ON run_snapshots
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable('runtime_snapshot');
CREATE TRIGGER run_snapshots_open_until_finished BEFORE UPDATE ON run_snapshots
    FOR EACH ROW EXECUTE FUNCTION enforce_run_snapshot_open();
