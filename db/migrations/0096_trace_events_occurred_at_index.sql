DROP INDEX trace_events_run_id_idx;

CREATE INDEX trace_events_occurred_at_idx ON trace_events (occurred_at);
