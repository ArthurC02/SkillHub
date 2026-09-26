CREATE TABLE publishers (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL UNIQUE REFERENCES workspaces (id),
    name         text NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE publications (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    publisher_id      uuid NOT NULL REFERENCES publishers (id),
    name              text NOT NULL,
    skill_id          uuid NOT NULL UNIQUE REFERENCES skills (id) ON DELETE CASCADE,
    status            text NOT NULL CHECK (status IN ('published', 'delisted')),
    status_changed_at timestamptz NOT NULL DEFAULT now(),
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (publisher_id, name)
);

CREATE TABLE publication_releases (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    publication_id   uuid NOT NULL REFERENCES publications (id) ON DELETE CASCADE,
    skill_version_id uuid NOT NULL REFERENCES skill_versions (id) ON DELETE CASCADE,
    version_number   integer NOT NULL,
    content_hash     text NOT NULL,
    findings         jsonb NOT NULL,
    rights_attested  boolean NOT NULL,
    released_by      uuid NOT NULL REFERENCES users (id),
    released_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX publication_releases_newest_idx
    ON publication_releases (publication_id, released_at DESC, id DESC);

CREATE TRIGGER publication_releases_immutable
    BEFORE UPDATE OR DELETE ON publication_releases
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();
