CREATE TABLE cost_events (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind              text NOT NULL CHECK (kind IN (
                          'creation_step', 'search_embedding', 'index_enrich',
                          'review', 'suggestion', 'generate')),
    model             text NOT NULL,
    prompt_version    text,
    prompt_tokens     bigint NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
    completion_tokens bigint NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0),
    usd_micros        bigint NOT NULL CHECK (usd_micros >= 0),
    cost_source       text NOT NULL CHECK (cost_source IN ('gateway', 'estimated')),
    workspace_id      uuid REFERENCES workspaces (id),
    user_id           uuid REFERENCES users (id),
    ref_type          text CHECK (ref_type IS NULL OR ref_type IN (
                          'creation_session', 'run', 'skill_version')),
    ref_id            uuid,
    idempotency_key   text NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cost_events_idempotency_key_key UNIQUE (idempotency_key)
);

CREATE INDEX cost_events_kind_created_at_idx ON cost_events (kind, created_at);
CREATE INDEX cost_events_user_created_at_idx ON cost_events (user_id, created_at)
    WHERE user_id IS NOT NULL;

CREATE TRIGGER cost_events_immutable
BEFORE UPDATE OR DELETE ON cost_events
FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

CREATE TABLE credit_accounts (
    user_id          uuid PRIMARY KEY REFERENCES users (id),
    balance_credits  bigint NOT NULL DEFAULT 0 CHECK (balance_credits >= -1000000),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE credit_entries (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL REFERENCES credit_accounts (user_id),
    kind            text NOT NULL CHECK (kind IN ('debit', 'grant', 'topup', 'adjustment')),
    delta_credits   bigint NOT NULL CHECK (delta_credits <> 0),
    usd_micros      bigint CHECK (usd_micros IS NULL OR usd_micros >= 0),
    markup_bps      integer CHECK (markup_bps IS NULL OR markup_bps >= 0),
    model           text,
    prompt_version  text,
    ref_type        text CHECK (ref_type IS NULL OR ref_type IN (
                        'creation_session', 'run', 'skill_version', 'operator_grant')),
    ref_id          uuid,
    cost_event_id   uuid REFERENCES cost_events (id),
    estimated       boolean NOT NULL DEFAULT false,
    idempotency_key text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT credit_entries_idempotency_key_key UNIQUE (idempotency_key),
    CONSTRAINT credit_entries_debit_has_cost CHECK (
        kind <> 'debit' OR (
            delta_credits < 0 AND usd_micros IS NOT NULL AND markup_bps IS NOT NULL
            AND cost_event_id IS NOT NULL
        )
    ),
    CONSTRAINT credit_entries_credit_kind_positive CHECK (
        kind NOT IN ('grant', 'topup') OR delta_credits > 0
    )
);

CREATE INDEX credit_entries_user_created_at_idx ON credit_entries (user_id, created_at DESC);

CREATE TRIGGER credit_entries_immutable
BEFORE UPDATE OR DELETE ON credit_entries
FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

CREATE TABLE cost_statistics (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind            text NOT NULL CHECK (kind IN (
                        'creation_step', 'search_embedding', 'index_enrich',
                        'review', 'suggestion', 'generate')),
    window_start    timestamptz NOT NULL,
    window_end      timestamptz NOT NULL CHECK (window_end > window_start),
    sample_count    bigint NOT NULL CHECK (sample_count >= 0),
    p50_usd_micros  bigint,
    p90_usd_micros  bigint,
    p95_usd_micros  bigint,
    max_usd_micros  bigint,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cost_statistics_kind_window_end_key UNIQUE (kind, window_end)
);

CREATE INDEX cost_statistics_kind_window_end_idx ON cost_statistics (kind, window_end DESC);
