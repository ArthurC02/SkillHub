CREATE TABLE dispatch_halts (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider    text NOT NULL DEFAULT '',
    source      text NOT NULL CHECK (source IN ('p1_incident', 'orphan_threshold')),
    reason      text NOT NULL CHECK (btrim(reason) <> ''),
    declared_by uuid REFERENCES users (id) ON DELETE SET NULL,
    declared_at timestamptz NOT NULL DEFAULT now(),
    clear_rounds integer NOT NULL DEFAULT 0 CHECK (clear_rounds >= 0),
    lifted_at   timestamptz,
    lifted_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    lift_reason text,
    CHECK ((lifted_at IS NULL) = (lift_reason IS NULL))
);

CREATE UNIQUE INDEX dispatch_halts_active_target_idx
    ON dispatch_halts (provider) WHERE lifted_at IS NULL;

COMMENT ON TABLE dispatch_halts IS
    'Active rows stop new Runs being dispatched (03:SEC-012 P1 first action, ADR-022 X-04 drain/suspend). provider = '''' is the whole pool. Shared by both triggers on purpose: one state, one release path. See 0030.';
