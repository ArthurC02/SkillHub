-- 0006_auth: external identities and server-side sessions (CORE-005, ADR-020).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

-- Identity is keyed by (provider, provider_user_id), never by email: emails are
-- mutable and re-registrable at the provider. One row per provider per user.
CREATE TABLE user_identities (
    user_id          uuid NOT NULL REFERENCES users (id),
    provider         text NOT NULL,
    provider_user_id text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, provider_user_id)
);

CREATE UNIQUE INDEX user_identities_user_provider_key
    ON user_identities (user_id, provider);

-- Server-side sessions (ADR-020): the cookie holds a random token, the row holds
-- its SHA-256 so a database leak does not yield usable sessions. Logout and account
-- deletion revoke by deleting rows; expiry cleanup is idempotent batch delete.
CREATE TABLE sessions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    -- ponytail: fixed 30-day absolute expiry, no sliding renewal; add a
    -- last_seen_at + renewal path only if retention data asks for it.
    expires_at timestamptz NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
