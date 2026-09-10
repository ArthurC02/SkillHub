CREATE TABLE analytics_events (
    event_id     uuid NOT NULL DEFAULT gen_random_uuid(),
    event_name   text NOT NULL CHECK (event_name IN (
        'search_performed',      -- funnel 1: an intent was submitted
        'skill_detail_viewed',   -- funnel 1: at least one detail was opened
        'session_started',       -- funnel 7: came back after first use
        'download_started'       -- funnel 6's first half; the success half is download_records
    )),
    occurred_at  timestamptz NOT NULL DEFAULT now(),
    session_id   text NOT NULL,
    workspace_id uuid REFERENCES workspaces (id) ON DELETE SET NULL,

    query_length   integer CHECK (query_length IS NULL OR query_length >= 0),
    query_language text,   -- coarse bucket ('han', 'latin', 'mixed'), not a locale
    result_count   integer CHECK (result_count IS NULL OR result_count >= 0),
    has_results    boolean,
    filters_applied boolean,
    skill_id     uuid,     -- the platform's own public identifier
    arrival      text CHECK (arrival IS NULL OR arrival IN ('search', 'direct')),
    arrival_rank integer CHECK (arrival_rank IS NULL OR arrival_rank >= 1),
    artifact_id  uuid,
    target       text,

    PRIMARY KEY (event_id, occurred_at)
) PARTITION BY RANGE (occurred_at);

CREATE INDEX analytics_events_session_idx ON analytics_events (session_id, occurred_at);
CREATE INDEX analytics_events_name_idx ON analytics_events (event_name, occurred_at);

CREATE TABLE analytics_events_2026_08 PARTITION OF analytics_events
    FOR VALUES FROM ('2026-08-01 00:00:00+00') TO ('2026-09-01 00:00:00+00');

CREATE TABLE analytics_events_default PARTITION OF analytics_events DEFAULT;

CREATE TABLE feedback_reports (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid REFERENCES workspaces (id) ON DELETE SET NULL,
    user_id      uuid REFERENCES users (id) ON DELETE SET NULL,
    kind         text NOT NULL CHECK (kind IN ('blocking_issue', 'need_signal')),
    message      text NOT NULL CHECK (length(message) BETWEEN 1 AND 2000),
    page_path    text CHECK (page_path IS NULL OR length(page_path) <= 512),
    run_id       uuid REFERENCES runs (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX feedback_reports_kind_idx ON feedback_reports (kind, created_at);
