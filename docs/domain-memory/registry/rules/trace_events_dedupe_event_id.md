---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0031_trace_event_id_dedupe.sql#L26-28"
    excerpt_sha256: "sha256:dddaabb25be69c4025a10a8ce54088084478eec367eaaa24f8d430898cc83a84"
---
trace_events keeps trigger trace_events_dedupe_event_id: CREATE TRIGGER trace_events_dedupe_event_id BEFORE INSERT ON trace_events FOR EACH ROW EXECUTE FUNCTION dedupe_trace_event_id()

# Relations
- Contexts: [trace](/contexts/trace)
