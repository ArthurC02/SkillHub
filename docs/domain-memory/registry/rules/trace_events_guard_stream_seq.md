---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0032_run_state_and_trace_stream_guards.sql#L64-66"
    excerpt_sha256: "sha256:29c9544b1828dc081c88a864dee7e101638ea0d2906ffdf52bc7e7f15ce14e53"
---
trace_events keeps trigger trace_events_guard_stream_seq: CREATE TRIGGER trace_events_guard_stream_seq BEFORE INSERT ON trace_events FOR EACH ROW EXECUTE FUNCTION enforce_trace_stream_seq()

# Relations
- Contexts: [trace](/contexts/trace)
