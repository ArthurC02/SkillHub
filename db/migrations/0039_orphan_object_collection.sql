CREATE TABLE object_collection_queue (
    object_key   text PRIMARY KEY,
    enqueued_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX object_collection_queue_enqueued_at_idx
    ON object_collection_queue (enqueued_at);

COMMENT ON TABLE object_collection_queue IS
    'Package object keys whose last referencing skill_versions row was deleted (04 丙-73). '
    'A key is removed from storage only when no skill_versions row references it, because '
    'the object is shared with every fork of the same content.';
