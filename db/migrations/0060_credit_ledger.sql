-- 0060_credit_ledger: platform credit accounting (CRED-001..CRED-009,
-- ADR-068).
-- 05 (2026-09-08 裁定)：this system runs on Credit only, and Credit only spends
-- here. Every paid call (creation step, catalog search embedding, index
-- enrichment, evaluation judge/suggest, single-generation comparison) is
-- metered in `cost_events` (the platform's real USD spend, in micro-dollars
-- so no float ever represents money); every balance-changing event for a
-- user is recorded in
-- `credit_entries` (the user-facing ledger: debit/grant/topup/adjustment).
-- These are two separate books on purpose — folding them into one table would
-- let an anonymous cost row (no user) collide with the per-account ledger
-- invariant this migration enforces. `credit_accounts` holds the one
-- materialized balance per user that reads use; `credit_entries` is the
-- source of truth reconciliation sums against. `cost_statistics` holds the
-- rolling-window percentiles the gate-before-session threshold is derived
-- from.
--
-- Applied migrations are immutable: fix forward with a new file, never edit
-- this one.

-- cost_events: the platform's own spend, one row per billable model call.
-- Never stores query text (ADR-029's "no query content in analytics events"
-- rule applies the same way to cost events). workspace_id/user_id are both
-- nullable — an anonymous catalog search still costs an embedding call.
CREATE TABLE cost_events (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Exact vocabulary from ADR-068 決策 3: creator/creation's settleCost,
    -- skill/discovery's search embedding, catalog index enrichment,
    -- trial/improvement's judge ("review") and suggest-improvements
    -- ("suggestion"), and skill/admission's single-generation comparison
    -- ("generate", `/v1/generate-skill`).
    kind              text NOT NULL CHECK (kind IN (
                          'creation_step', 'search_embedding', 'index_enrich',
                          'review', 'suggestion', 'generate')),
    model             text NOT NULL,
    prompt_version    text,
    prompt_tokens     bigint NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
    completion_tokens bigint NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0),
    -- Micro-dollars (1 usd_micros = $0.000001): bigint, never float, per the
    -- brief's money rule.
    usd_micros        bigint NOT NULL CHECK (usd_micros >= 0),
    cost_source       text NOT NULL CHECK (cost_source IN ('gateway', 'estimated')),
    workspace_id      uuid REFERENCES workspaces (id),
    user_id           uuid REFERENCES users (id),
    -- "三選一" (ADR-068 決策 3): session id / run id / skill version id, or
    -- NULL for a call with none of the three (e.g. an anonymous search).
    ref_type          text CHECK (ref_type IS NULL OR ref_type IN (
                          'creation_session', 'run', 'skill_version')),
    ref_id            uuid,
    -- Caller-assigned so a retried gateway call records exactly once; format is
    -- the caller's (e.g. "creation:<session_id>:<revision>:<call>").
    idempotency_key   text NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cost_events_idempotency_key_key UNIQUE (idempotency_key)
);

-- Rolling-window aggregation (event-driven at session end, and River's daily
-- recompute) reads by kind and time; both use this.
CREATE INDEX cost_events_kind_created_at_idx ON cost_events (kind, created_at);
-- Per-user/workspace cost views (support, disputes) filter this way.
CREATE INDEX cost_events_user_created_at_idx ON cost_events (user_id, created_at)
    WHERE user_id IS NOT NULL;

-- Append-only like evaluation_model_usage (0033): a spend record that could be
-- edited after the fact is not a record. DELETE stays reachable for the
-- retention sweep the same way audit_events' does (0013's enforce_immutable,
-- gated on `SET LOCAL skillhub.purge = 'on'`).
CREATE TRIGGER cost_events_immutable
BEFORE UPDATE OR DELETE ON cost_events
FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

-- credit_accounts: one materialized balance per user (the identity root, not
-- workspace — 05's "每個帳號" is the user, and MVP's 1:1 user/workspace
-- doesn't change that). Read path uses this column directly; reconciliation
-- sums credit_entries.delta_credits instead.
CREATE TABLE credit_accounts (
    user_id          uuid PRIMARY KEY REFERENCES users (id),
    -- The debt floor the owner set (-50) is NOT this constraint, and putting it
    -- here was the first version's mistake. Gate ② checks the floor before a
    -- step is paid for; the charge that follows is for money already spent, and
    -- ADR-068 decision 7 says it lands whatever the balance is. A CHECK on the
    -- same number would refuse that write, roll back the transaction that also
    -- carries the session snapshot, and leave real spend recorded nowhere -
    -- worse than a balance of -53. This rail is far below any configured floor:
    -- it catches a runaway loop or a sign error, not a person who overspent.
    balance_credits  bigint NOT NULL DEFAULT 0 CHECK (balance_credits >= -1000000),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

-- credit_entries: the append-only ledger. Every balance change — debit against
-- a cost_events row, operator grant/topup, manual adjustment — is one row
-- here, and credit_accounts.balance_credits is updated in the same
-- transaction as the insert (see queries/credit.sql). References
-- credit_accounts rather than users directly so an entry can never exist
-- without the account row its balance update targets (callers run
-- EnsureCreditAccount first).
CREATE TABLE credit_entries (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL REFERENCES credit_accounts (user_id),
    kind            text NOT NULL CHECK (kind IN ('debit', 'grant', 'topup', 'adjustment')),
    -- Signed: negative for debit, positive for grant/topup, either sign for a
    -- manual adjustment (a correction can go either way).
    delta_credits   bigint NOT NULL CHECK (delta_credits <> 0),
    -- The USD spend and markup this entry represents. NULL for grant/topup/
    -- adjustment rows that carry no gateway cost; required for every debit so
    -- a debit can never be silently free.
    usd_micros      bigint CHECK (usd_micros IS NULL OR usd_micros >= 0),
    -- Basis points at the time of this entry (13000 = 1.3x). Recorded per
    -- row, never overwritten, so a later markup change never rewrites
    -- history (mirrors ADR-003's "new version, not overwrite" rule).
    markup_bps      integer CHECK (markup_bps IS NULL OR markup_bps >= 0),
    model           text,
    prompt_version  text,
    ref_type        text CHECK (ref_type IS NULL OR ref_type IN (
                        'creation_session', 'run', 'skill_version', 'operator_grant')),
    ref_id          uuid,
    -- Which cost_events row this debit pays for. Required for debit, absent
    -- for grant/topup/adjustment (those have no metered call behind them).
    cost_event_id   uuid REFERENCES cost_events (id),
    -- True when usage was unknown and this debit charged the reserved amount
    -- instead (05: "絕不因讀不到成本而扣 0").
    estimated       boolean NOT NULL DEFAULT false,
    -- Caller-assigned; creation-step debits use "(session_id, revision)" so a
    -- retried settlement never double-charges.
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

-- "依 user 取... 最近分錄" (05).
CREATE INDEX credit_entries_user_created_at_idx ON credit_entries (user_id, created_at DESC);

CREATE TRIGGER credit_entries_immutable
BEFORE UPDATE OR DELETE ON credit_entries
FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

-- cost_statistics: rolling-window percentiles cost_events is aggregated into,
-- read back to derive the before-session and per-step gates (05: "門檻由統計
-- 推導"). Not itself immutable — mistaken/short-sample windows are expected
-- to be superseded by a fresh recompute of the same (kind, window_end), and a
-- derived summary table is not the ledger ADR-003 is about.
CREATE TABLE cost_statistics (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Same vocabulary as cost_events.kind (kept as a literal CHECK, not a FK
    -- to a lookup table, for the same reason cost_events' is: six kinds
    -- across four contexts share this list by convention already).
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

-- "讀最新統計": latest row per kind.
CREATE INDEX cost_statistics_kind_window_end_idx ON cost_statistics (kind, window_end DESC);
