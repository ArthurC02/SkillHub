ALTER TABLE platform_agent_runs ADD COLUMN result jsonb;

CREATE TABLE platform_agent_steps (
    run_id            uuid NOT NULL REFERENCES platform_agent_runs (id),
    seq               integer NOT NULL CHECK (seq >= 0),
    tool              text NOT NULL,
    arguments         text NOT NULL,
    result            text NOT NULL,
    model             text NOT NULL,
    prompt_tokens     bigint NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
    completion_tokens bigint NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0),
    usd_micros        bigint CHECK (usd_micros >= 0),
    created_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, seq)
);

ALTER TABLE cost_events DROP CONSTRAINT cost_events_kind_check;
ALTER TABLE cost_events ADD CONSTRAINT cost_events_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run', 'match_reasons', 'suggest_criteria',
    'search_intent', 'platform_agent'));

ALTER TABLE cost_statistics DROP CONSTRAINT cost_statistics_kind_check;
ALTER TABLE cost_statistics ADD CONSTRAINT cost_statistics_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run', 'creation_session', 'match_reasons',
    'suggest_criteria', 'search_intent', 'platform_agent'));

ALTER TABLE cost_events DROP CONSTRAINT cost_events_ref_type_check;
ALTER TABLE cost_events ADD CONSTRAINT cost_events_ref_type_check CHECK (ref_type IS NULL OR ref_type IN (
    'creation_session', 'run', 'skill_version', 'platform_agent_run'));
