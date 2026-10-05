ALTER TABLE outbox_events
    ADD COLUMN next_delivery_at timestamptz;
