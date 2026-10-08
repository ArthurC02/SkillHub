CREATE TABLE platform_agent_findings (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id          uuid NOT NULL REFERENCES platform_agents (id),
    status            text NOT NULL DEFAULT 'open'
                      CHECK (status IN ('open', 'acknowledged', 'resolved', 'dismissed', 'recovered')),
    title             text NOT NULL CHECK (btrim(title) <> ''),
    cites             text[] NOT NULL CHECK (cardinality(cites) > 0),
    assignee_id       uuid REFERENCES users (id),
    first_seen_at     timestamptz NOT NULL DEFAULT now(),
    last_seen_at      timestamptz NOT NULL DEFAULT now(),
    seen_count        integer NOT NULL DEFAULT 1 CHECK (seen_count > 0),
    status_changed_at timestamptz NOT NULL DEFAULT now(),
    CHECK (status <> 'acknowledged' OR assignee_id IS NOT NULL)
);

CREATE INDEX platform_agent_findings_status_seen_idx ON platform_agent_findings (status, last_seen_at DESC);
CREATE INDEX platform_agent_findings_agent_seen_idx ON platform_agent_findings (agent_id, last_seen_at DESC);

CREATE TABLE platform_agent_finding_events (
    finding_id  uuid NOT NULL REFERENCES platform_agent_findings (id),
    seq         integer NOT NULL CHECK (seq >= 0),
    kind        text NOT NULL
                CHECK (kind IN ('opened', 'seen', 'reopened', 'recovered', 'acknowledged', 'resolved', 'dismissed')),
    run_id      uuid REFERENCES platform_agent_runs (id),
    operator_id uuid REFERENCES users (id),
    text        text,
    evidence    jsonb,
    note        text,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (finding_id, seq),
    CHECK (num_nonnulls(run_id, operator_id) = 1),
    CHECK (operator_id IS NULL OR btrim(note) <> '')
);
