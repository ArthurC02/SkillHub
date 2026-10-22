---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0019_trace_ingestion.sql#L3-3"
    excerpt_sha256: "sha256:f282dae96f543bb9992c5fe78937f3e597157aedb3326baa60190e89add338be"
---
trace_events keeps unique-index trace_events_event_id_key: CREATE UNIQUE INDEX trace_events_event_id_key ON trace_events (event_id, occurred_at)

# Relations
- Contexts: [trace](/contexts/trace)
