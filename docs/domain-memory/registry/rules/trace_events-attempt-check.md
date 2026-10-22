---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0019_trace_ingestion.sql#L5-6"
    excerpt_sha256: "sha256:9bd47871cdc9e3aa24d8787b975e378abbb25866efc837105adc47ced670227e"
---
trace_events keeps check trace_events.attempt: attempt integer NOT NULL DEFAULT 1 CHECK (attempt >= 1)

# Relations
- Contexts: [trace](/contexts/trace)
