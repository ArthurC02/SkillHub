---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0034_trace_incremental_reads.sql#L29-31"
    excerpt_sha256: "sha256:572b18145fd0eb26e20d01cc9f0674d8da80662ff9d650d4d9c481ae0bf62d42"
---
trace_events keeps trigger trace_events_assign_ingest_seq: CREATE TRIGGER trace_events_assign_ingest_seq BEFORE INSERT ON trace_events FOR EACH ROW EXECUTE FUNCTION assign_trace_ingest_seq()

# Relations
- Contexts: [trace](/contexts/trace)
