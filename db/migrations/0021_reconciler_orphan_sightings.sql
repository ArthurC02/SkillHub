-- 0021_reconciler_orphan_sightings: the Reconciler's in-flight orphan table
-- (ADR-022 X-03/X-04, 03:SBX-012). Applied migrations are immutable: fix forward
-- with a new file, never edit this one.
--
-- Why a table and not an in-memory map: X-03's threshold is "the same
-- provider_run_id is still there two consecutive rounds", and the Reconciler is a
-- River leader-only periodic job. Leadership moves between worker processes and
-- processes restart, so a map would reset the round count exactly when the leak it
-- is counting outlives a worker — which is the case the threshold exists for.
--
-- No workspace_id, and that is not an iron-rule-3 exception: this is the execution
-- plane's own reconciliation state, keyed by a provider handle. No user input
-- reaches it and no user data leaves it (same reasoning as GetRunAttemptForReconcile).
--
-- provider_run_id is the provider's temporary handle, never an identity (iron rule
-- 10): it is the key here because the thing being counted is a provider-side
-- resource that the platform, by definition, has no run for any more.
CREATE TABLE reconciler_orphan_sightings (
    provider        text NOT NULL,
    provider_run_id text NOT NULL,
    -- The round this resource was first judged leaked. Kept alongside the count so
    -- an operator can see how long it has been there, not just how many rounds.
    first_seen_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    -- Consecutive rounds, not total sightings: a round that does not see this
    -- resource deletes the row, so a later reappearance starts over at 1. That is
    -- what makes `rounds >= 2` mean "still present two rounds running" rather than
    -- "seen twice at some point".
    rounds          integer NOT NULL DEFAULT 1 CHECK (rounds > 0),
    PRIMARY KEY (provider, provider_run_id)
);
