---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0005_immutability.sql#L49-51"
    excerpt_sha256: "sha256:0c6855b66870d37a7a9a0094da7c4cbb0865ffcee3425b4aba12b4acf46495f7"
---
trace_events keeps trigger trace_events_immutable: CREATE TRIGGER trace_events_immutable BEFORE UPDATE OR DELETE ON trace_events FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [trace](/contexts/trace)
