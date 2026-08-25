-- 0039_orphan_object_collection: remember a package object's key at the moment
-- the last row that knew about it is deleted (04 丙-73).
--
-- The gap this closes was articulated inside the codebase before it was noticed:
-- purgeAccount's own comment explains why it removes objects BEFORE its
-- transaction, and names the alternative exactly — "the other order can leave a
-- user's uploaded file alive with nothing left in the database that knows it
-- exists."
--
-- Package objects cannot use that order. They are content-addressed and shared
-- with every fork, so whether an object may go is only knowable AFTER the rows
-- are gone: while `skill_versions` still holds a row with that key, somebody
-- else may be reading it. Rows first, objects second — which opens precisely the
-- window purgeAccount avoided, and this table is what closes it.
--
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

CREATE TABLE object_collection_queue (
    -- The key is the identity. A skill and its fork share one content-addressed
    -- object, so two version rows deleted in the same sweep enqueue the same key
    -- twice; the primary key makes the second one a no-op instead of a duplicate
    -- unit of work (iron rule 9 — enqueueing has to be safe to repeat).
    object_key   text PRIMARY KEY,
    enqueued_at  timestamptz NOT NULL DEFAULT now()
);

-- The sweep reads oldest-first and is bounded, so a backlog drains over several
-- runs rather than in one long transaction.
CREATE INDEX object_collection_queue_enqueued_at_idx
    ON object_collection_queue (enqueued_at);

-- Deliberately NOT frozen by the 0005 trigger, unlike almost everything else a
-- purge touches. This table is a worklist, not a record of anything that
-- happened: a row appears when an object becomes a candidate and disappears when
-- it has been dealt with, and both directions are ordinary work. Freezing it
-- would mean the sweep needed the `skillhub.purge` exemption to finish its own
-- job, which is the exemption doing the opposite of what it is for.
--
-- What is NOT recorded here, on purpose: which skill or workspace the key came
-- from. By the time a row lands the owner is already gone, and keeping the
-- association would turn a garbage-collection worklist into a record of what a
-- deleted account used to hold — the shape CORE-007 and PDM-006 §6.1 exist to
-- prevent. A bare content-addressed key names bytes, not a person.
COMMENT ON TABLE object_collection_queue IS
    'Package object keys whose last referencing skill_versions row was deleted (04 丙-73). '
    'A key is removed from storage only when no skill_versions row references it, because '
    'the object is shared with every fork of the same content.';
