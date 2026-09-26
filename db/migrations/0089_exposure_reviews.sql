ALTER TABLE search_documents ADD COLUMN exposure_digest text GENERATED ALWAYS AS (
    md5(coalesce(latest_version_id::text, '') || chr(31) || name || chr(31) || summary || chr(31) ||
        enriched_summary || chr(31) || task_examples || chr(31) || coalesce(tags::text, '') || chr(31) ||
        limitations)
) STORED;

CREATE TABLE exposure_reviews (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    publication_id   uuid NOT NULL REFERENCES publications (id) ON DELETE CASCADE,
    sequence         integer NOT NULL CHECK (sequence > 0),
    release_id       uuid NOT NULL REFERENCES publication_releases (id) ON DELETE CASCADE,
    content_hash     text NOT NULL,
    snapshot_digest  text NOT NULL,
    decision         text NOT NULL CHECK (decision IN ('approved', 'revoked')),
    reason           text NOT NULL CHECK (btrim(reason) <> ''),
    reviewer_user_id uuid NOT NULL REFERENCES users (id),
    reviewed_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (publication_id, sequence)
);

CREATE TRIGGER exposure_reviews_immutable
    BEFORE UPDATE OR DELETE ON exposure_reviews
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();
