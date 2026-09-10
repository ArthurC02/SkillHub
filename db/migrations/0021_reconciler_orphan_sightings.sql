CREATE TABLE reconciler_orphan_sightings (
    provider        text NOT NULL,
    provider_run_id text NOT NULL,
    first_seen_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    rounds          integer NOT NULL DEFAULT 1 CHECK (rounds > 0),
    PRIMARY KEY (provider, provider_run_id)
);
