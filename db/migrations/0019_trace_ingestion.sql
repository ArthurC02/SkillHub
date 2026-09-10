ALTER TABLE trace_events ADD COLUMN event_id uuid NOT NULL;

CREATE UNIQUE INDEX trace_events_event_id_key ON trace_events (event_id, occurred_at);

ALTER TABLE trace_events ADD COLUMN attempt integer NOT NULL DEFAULT 1
    CHECK (attempt >= 1);

ALTER TABLE trace_events ADD COLUMN schema_version text NOT NULL DEFAULT '1.0';

ALTER TABLE trace_events
    ADD COLUMN masked boolean NOT NULL DEFAULT false CHECK (masked),
    ADD COLUMN masked_fields jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE trace_events ADD COLUMN late boolean NOT NULL DEFAULT false;

CREATE INDEX trace_events_stream_idx ON trace_events (run_id, attempt, source, seq);

CREATE TABLE trace_events_default PARTITION OF trace_events DEFAULT;
