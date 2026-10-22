---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0032_run_state_and_trace_stream_guards.sql#L24-26"
    excerpt_sha256: "sha256:6a6e68da195051375df9c00a86a97e88f65a1c6f38c0a8186a27b949374093c9"
---
runs keeps trigger runs_validate_status_transition: CREATE TRIGGER runs_validate_status_transition BEFORE UPDATE OF status ON runs FOR EACH ROW EXECUTE FUNCTION enforce_run_status_transition()

# Relations
- Contexts: [run](/contexts/run)
