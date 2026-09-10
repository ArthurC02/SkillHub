ALTER TABLE artifacts ADD COLUMN purged_at timestamptz;

COMMENT ON COLUMN artifacts.purged_at IS
    'When the stored object was removed while the row was kept: retention expiry, or a reconciler finding the bytes gone. Distinct from deleted_at, which is the owner deleting it and hides the row (SEC-006). Serving the bytes requires this to be NULL. See 0028.';

CREATE INDEX artifacts_unpurged_expiry_idx ON artifacts (expires_at)
    WHERE purged_at IS NULL AND deleted_at IS NULL;

CREATE TABLE object_reconcile_sightings (
    resource_kind text NOT NULL CHECK (resource_kind IN ('dataset', 'artifact')),
    resource_id   uuid NOT NULL,
    object_key    text NOT NULL,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now(),
    rounds        integer NOT NULL DEFAULT 1 CHECK (rounds > 0),
    PRIMARY KEY (resource_kind, resource_id)
);
