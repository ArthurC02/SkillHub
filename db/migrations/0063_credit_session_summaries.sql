-- One row per creation session that paid for at least one step: the sample the start gate's p95 is taken over.
CREATE TABLE cost_session_summaries (
    session_id   uuid PRIMARY KEY,
    user_id      uuid REFERENCES users (id),
    usd_micros   bigint NOT NULL CHECK (usd_micros >= 0),
    steps        integer NOT NULL CHECK (steps >= 1),
    estimated    boolean NOT NULL,
    last_step_at timestamptz NOT NULL
);
CREATE INDEX cost_session_summaries_last_step_at ON cost_session_summaries (last_step_at);

ALTER TABLE cost_statistics DROP CONSTRAINT cost_statistics_kind_check;
ALTER TABLE cost_statistics ADD CONSTRAINT cost_statistics_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run', 'creation_session'));
