---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0019_trace_ingestion.sql#L10-12"
    excerpt_sha256: "sha256:390b97b8e5e15d22ab4a95e188b63fba4d97239419ef3aff1d32d6c996c12a50"
---
trace_events keeps check trace_events.masked: masked boolean NOT NULL DEFAULT false CHECK (masked)

# Relations
- Contexts: [trace](/contexts/trace)
