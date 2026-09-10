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

ALTER TABLE outbox_events
    ADD COLUMN delivery_attempts integer     NOT NULL DEFAULT 0,
    ADD COLUMN dead_lettered_at  timestamptz;

DROP INDEX outbox_events_unpublished_idx;
CREATE INDEX outbox_events_unpublished_idx ON outbox_events (occurred_at)
    WHERE published_at IS NULL AND dead_lettered_at IS NULL;

CREATE INDEX outbox_events_published_idx ON outbox_events (published_at)
    WHERE published_at IS NOT NULL;
