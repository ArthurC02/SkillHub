-- 0002_identity: users and personal workspaces (CORE-001).
-- Workspace is the tenancy boundary for every user-owned row (ADR-011, iron rule 3);
-- `users` is the identity root and is therefore the only table without workspace_id.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

CREATE TABLE users (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email        text NOT NULL,
    display_name text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    -- Account deletion is a 30-day grace period before private content is hard deleted
    -- and referenced immutable rows are de-identified (PDM-006 6.1). Rows referencing a
    -- user are never rewritten, so de-identification happens here, not on the versions.
    deleted_at   timestamptz
);

CREATE UNIQUE INDEX users_email_key ON users (lower(email)) WHERE deleted_at IS NULL;

CREATE TABLE workspaces (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id uuid NOT NULL REFERENCES users (id),
    name          text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- MVP gives every user exactly one personal workspace (ADR-011). Nothing else in the
-- schema assumes 1:1, so team workspaces only need this index dropped.
CREATE UNIQUE INDEX workspaces_owner_user_id_key ON workspaces (owner_user_id);
