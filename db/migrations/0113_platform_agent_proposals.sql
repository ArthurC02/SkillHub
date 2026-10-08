CREATE TABLE platform_agent_proposals (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      uuid NOT NULL REFERENCES platform_agents (id),
    run_id        uuid NOT NULL REFERENCES platform_agent_runs (id),
    action        text NOT NULL CHECK (btrim(action) <> ''),
    tier          text NOT NULL CHECK (tier IN ('read_only', 'reversible', 'destructive')),
    reason        text NOT NULL CHECK (btrim(reason) <> ''),
    cites         text[] NOT NULL CHECK (cardinality(cites) > 0),
    preview       jsonb NOT NULL,
    status        text NOT NULL DEFAULT 'proposed'
                  CHECK (status IN ('proposed', 'approved', 'rejected', 'expired', 'running', 'succeeded', 'failed')),
    proposed_at   timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL,
    decided_by    uuid REFERENCES users (id),
    decided_at    timestamptz,
    decision_note text,
    started_at    timestamptz,
    finished_at   timestamptz,
    outcome       text,
    CHECK (expires_at > proposed_at),
    CHECK ((status IN ('proposed', 'expired')) = (decided_by IS NULL)),
    CHECK ((decided_by IS NULL) = (decided_at IS NULL)),
    CHECK (decided_by IS NULL OR btrim(decision_note) <> ''),
    CHECK ((status IN ('running', 'succeeded', 'failed')) = (started_at IS NOT NULL)),
    CHECK ((status IN ('rejected', 'expired', 'succeeded', 'failed')) = (finished_at IS NOT NULL)),
    CHECK (status <> 'failed' OR btrim(outcome) <> '')
);

CREATE UNIQUE INDEX platform_agent_proposals_one_live_idx ON platform_agent_proposals (action)
    WHERE status IN ('proposed', 'approved', 'running');
CREATE INDEX platform_agent_proposals_status_idx ON platform_agent_proposals (status, proposed_at DESC);

GRANT SELECT, UPDATE ON platform_agent_proposals TO skillhub_purge;
