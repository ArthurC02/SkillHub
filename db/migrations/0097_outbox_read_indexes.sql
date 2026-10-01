DROP INDEX outbox_events_aggregate_idx;

CREATE INDEX outbox_events_type_occurred_idx ON outbox_events (event_type, occurred_at);

CREATE INDEX outbox_events_dead_lettered_idx ON outbox_events (dead_lettered_at)
    WHERE dead_lettered_at IS NOT NULL;
