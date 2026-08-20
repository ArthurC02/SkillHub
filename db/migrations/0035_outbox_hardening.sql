-- 0035_outbox_hardening: close the transactional outbox gaps ADR-008 named and
-- 0016 left open (contracts/events/domain-events.md §5 缺口 1/2/4/5/7, DDD-012).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- Three holes, one file, because they are the same hole seen from three sides —
-- the table was a buffer with no floor, no ceiling and no vocabulary:
--
--   * event_type was open text, so a producer's string concatenation could mint a
--     new event type that no catalogue lists and no consumer routes on;
--   * a delivery that fails every time blocked everything committed behind it,
--     for as long as it kept failing, which ADR-008 forbids by name
--     (「Poison Message 進入隔離佇列並告警，不無限制重送」);
--   * 0016 assumed the publisher would delete drained rows. It never did, so the
--     buffer was accumulating history it is explicitly not the keeper of — the
--     400-day trail is audit_events, not this table.

-- 缺口 7 / §4 rule 2: the value set is closed here as well as in Go, because the
-- Go constants cannot see a hand-written INSERT and a hand-written INSERT is how
-- a maintenance script gets a typo into a consumer's routing.
ALTER TABLE outbox_events
    ADD CONSTRAINT outbox_events_event_type_check CHECK (event_type IN (
        'run.queued',
        'run.provisioning',
        'run.preparing',
        'run.running',
        'run.evaluating',
        'run.succeeded',
        'run.failed',
        'run.cancelled',
        'run.timed_out',
        'run.cleanup_cleaned',
        'run.cleanup_failed'
    ));

-- 缺口 2: attempts counted on the row, so the count survives the publisher that
-- did the counting. dead_lettered_at is set once, and the row is then left alone
-- for a human — deleting it would destroy the evidence of why it could not be
-- delivered, which is the only thing it is still good for.
ALTER TABLE outbox_events
    ADD COLUMN delivery_attempts integer     NOT NULL DEFAULT 0,
    ADD COLUMN dead_lettered_at  timestamptz;

-- The publisher's scan now skips isolated rows, so its index has to skip them
-- too or it stops covering the only query it exists for. Replaces 0016's.
DROP INDEX outbox_events_unpublished_idx;
CREATE INDEX outbox_events_unpublished_idx ON outbox_events (occurred_at)
    WHERE published_at IS NULL AND dead_lettered_at IS NULL;

-- 缺口 1: retention runs on every publish pass, so it must find its rows by
-- published_at rather than re-reading the table each tick. Partial for the same
-- reason as the scan above: unpublished rows are never retention's business.
CREATE INDEX outbox_events_published_idx ON outbox_events (published_at)
    WHERE published_at IS NOT NULL;
