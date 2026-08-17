-- 0029_beta_analytics: the two things the closed beta needs a table for — the
-- product analytics events of the core funnel (O11Y-004, ADR-029) and the
-- qualitative feedback its three entry points collect (BETA-003/004/005).
-- Shape and reasoning: docs/plans/mvp/m4/beta-design.md §4, §5, ADR-029.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- What this migration deliberately does NOT add: a quota balance column. The run
-- allowance of PDM-010 is enforced by counting the runs themselves in the same
-- transaction that creates one (ADR-028 決策 2), because a balance is a cache that
-- drifts and a refund would be a write somebody can miss. There is nothing to
-- store, so nothing is stored — see internal/run/quota.go.

-- (a) product analytics events -------------------------------------------------
-- The fifth data class of ADR-029, and the only one that is explicitly NOT a
-- source of truth. Every funnel segment an existing domain table can answer is
-- answered there (runs, evaluations.feedback_helpful,
-- evaluation_suggestions.decision, download_records); only the four segments no
-- table can answer land here. A fifth event name has to justify itself against
-- that rule first, which is why the CHECK below is closed rather than open.
--
-- Iron rule 11 says secrets must not appear in "分析事件", and until ADR-029 that
-- word had no referent anywhere in the repo. It does now, and the guarantee is
-- structural rather than a masking pass: there is no free-text column in this
-- table. Not "masked free text" — none. An attribute a schema never declared is
-- dropped by the writer, because a blacklist that misses one entry leaks and a
-- whitelist that misses one entry is short a dimension (same rule the packaging
-- content filter is built on).
--
-- Deliberately NOT here, and each one is a decision rather than an omission:
--   * the query string        — it can carry personal data; length and language
--                               are recorded instead, and the words themselves
--                               only reach BETA-003's channel, which has consent
--   * a named actor           — audit answers "who did what" and keeps it 400 days
--                               unaltered; this answers "how many got to段 N" and
--                               may be deleted wholesale, which is exactly why one
--                               cannot stand in for the other (ADR-029 決策 3)
--   * the ADR-020 session token or its hash — that is credential material.
--     `session_id` here is an unrelated random value that cannot be resolved back
--     to a sessions row, and before login it is the only linkage there is.
CREATE TABLE analytics_events (
    event_id     uuid NOT NULL DEFAULT gen_random_uuid(),
    -- Closed set: ADR-029 決策 1 allows exactly these four, and adding a fifth is
    -- meant to require explaining why no domain table answers it. A CHECK makes
    -- that argument happen in review rather than after the fact.
    event_name   text NOT NULL CHECK (event_name IN (
        'search_performed',      -- funnel 1: an intent was submitted
        'skill_detail_viewed',   -- funnel 1: at least one detail was opened
        'session_started',       -- funnel 7: came back after first use
        'download_started'       -- funnel 6's first half; the success half is download_records
    )),
    occurred_at  timestamptz NOT NULL DEFAULT now(),
    -- The analytics session. Random, opaque, and not derived from anything in
    -- `sessions` (ADR-029 決策 4). Text rather than uuid because it is a client
    -- cookie value the platform never joins on.
    session_id   text NOT NULL,
    -- NULL until the visitor signs in — the first funnel segment happens before
    -- login and is measured anyway. ON DELETE SET NULL is also the de-identification
    -- ADR-029 決策 5 requires, for the day a workspace row is ever hard deleted;
    -- the purge as written anonymises rather than deletes, so identity.purgeAccount
    -- clears this column explicitly too.
    workspace_id uuid REFERENCES workspaces (id) ON DELETE SET NULL,

    -- Declared attributes. One column each rather than a jsonb bag: the whitelist
    -- is then the schema instead of a promise made in Go, and the funnel query is
    -- plain SQL. Every one is nullable because each belongs to one event name.
    --
    -- search_performed (ADR-029 決策 2: 型態, never content):
    query_length   integer CHECK (query_length IS NULL OR query_length >= 0),
    query_language text,   -- coarse bucket ('han', 'latin', 'mixed'), not a locale
    result_count   integer CHECK (result_count IS NULL OR result_count >= 0),
    has_results    boolean,
    filters_applied boolean,
    -- skill_detail_viewed:
    skill_id     uuid,     -- the platform's own public identifier
    arrival      text CHECK (arrival IS NULL OR arrival IN ('search', 'direct')),
    arrival_rank integer CHECK (arrival_rank IS NULL OR arrival_rank >= 1),
    -- download_started. No foreign key on purpose: an analytics row must outlive
    -- the artifact it describes (retention differs, and an expired download is
    -- still a funnel step that happened).
    artifact_id  uuid,
    target       text,

    PRIMARY KEY (event_id, occurred_at)
) PARTITION BY RANGE (occurred_at);

-- The funnel query reads a date range and groups by session; the retention job
-- drops whole partitions and needs no index at all.
CREATE INDEX analytics_events_session_idx ON analytics_events (session_id, occurred_at);
CREATE INDEX analytics_events_name_idx ON analytics_events (event_name, occurred_at);

-- Monthly range partitions, the form trace_events already uses (0004, 0019), for
-- the same reason: retention becomes DROP PARTITION rather than a bulk DELETE.
-- ADR-029 決策 5 proposes 180 days and that proposal is not ratified, so the
-- number lives in deployment configuration and not in this file — and until a
-- deployment sets it, the writer collects nothing at all (NFR-002: 未定值前不得
-- 開始收集).
CREATE TABLE analytics_events_2026_08 PARTITION OF analytics_events
    FOR VALUES FROM ('2026-08-01 00:00:00+00') TO ('2026-09-01 00:00:00+00');

-- ponytail: a DEFAULT partition catches every other month, exactly as 0019 does
-- for trace_events. Ceiling: attaching a real monthly partition later needs the
-- default to hold no rows for that month (detach, move, re-attach). Upgrade path
-- is the same scheduled pre-create job both tables will want once retention by
-- DROP PARTITION starts mattering.
CREATE TABLE analytics_events_default PARTITION OF analytics_events DEFAULT;

-- (b) beta feedback ------------------------------------------------------------
-- BETA-003/004/005. Three entry points, one table, split by `kind`: the visitor
-- who is not on the invite list and the one whose allowance ran out are asking the
-- same question ("what did you want that you could not have"), and PDM-010 §8.1
-- designed them to share a form; being stuck somewhere is a different question and
-- says so.
--
-- Not merged with analytics_events, and not with audit events. This one holds the
-- user's own words, so it is the opposite of the table above in every property
-- that matters: it has content, it is read one row at a time by a person, and it
-- has no aggregate reading at all.
--
-- Both owner columns are nullable with ON DELETE SET NULL because ADR-029 決策 5
-- keeps qualitative feedback de-identified rather than deleting it — the same
-- treatment PDM-006 §6.1 already gives run metadata. CASCADE here would quietly
-- destroy the evidence a scope review is based on.
CREATE TABLE feedback_reports (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid REFERENCES workspaces (id) ON DELETE SET NULL,
    user_id      uuid REFERENCES users (id) ON DELETE SET NULL,
    kind         text NOT NULL CHECK (kind IN ('blocking_issue', 'need_signal')),
    -- Bounded at the same 2000 characters public.yaml declares, so an oversized
    -- body is refused identically whether it arrives through the handler or not.
    message      text NOT NULL CHECK (length(message) BETWEEN 1 AND 2000),
    -- A route path, never a full URL: a query string can carry personal data and
    -- this is not the channel for it (beta-design §4.2 界線 2).
    page_path    text CHECK (page_path IS NULL OR length(page_path) <= 512),
    -- The run they were looking at, when there was one. Verified to be the
    -- caller's own before it is written, and dropped rather than refused when it
    -- is not — losing the report would be the worse outcome.
    run_id       uuid REFERENCES runs (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX feedback_reports_kind_idx ON feedback_reports (kind, created_at);
