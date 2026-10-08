CREATE TABLE platform_agents (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                   text NOT NULL UNIQUE CHECK (name ~ '^[a-z][a-z0-9-]*$'),
    purpose                text NOT NULL CHECK (btrim(purpose) <> ''),
    model_role             text NOT NULL CHECK (btrim(model_role) <> ''),
    daily_spend_cap_micros bigint NOT NULL CHECK (daily_spend_cap_micros > 0),
    tools                  text[] NOT NULL DEFAULT '{}',
    actions                text[] NOT NULL DEFAULT '{}',
    enabled                boolean NOT NULL DEFAULT false,
    owner_id               uuid REFERENCES users (id),
    registered_at          timestamptz NOT NULL DEFAULT now(),
    CHECK (NOT enabled OR owner_id IS NOT NULL)
);

CREATE TABLE platform_agent_brake (
    engaged    boolean PRIMARY KEY DEFAULT true CHECK (engaged),
    engaged_by uuid NOT NULL REFERENCES users (id),
    engaged_at timestamptz NOT NULL DEFAULT now(),
    reason     text NOT NULL CHECK (btrim(reason) <> '')
);

CREATE TABLE platform_agent_runs (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id    uuid NOT NULL REFERENCES platform_agents (id),
    status      text NOT NULL DEFAULT 'running'
                CHECK (status IN ('running', 'completed', 'incomplete', 'stopped', 'failed')),
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    reason      text,
    CHECK ((status = 'running') = (finished_at IS NULL)),
    CHECK (finished_at IS NULL OR status = 'completed' OR btrim(reason) <> '')
);

CREATE INDEX platform_agent_runs_agent_started_idx ON platform_agent_runs (agent_id, started_at DESC);
