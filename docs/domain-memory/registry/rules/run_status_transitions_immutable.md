---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0005_immutability.sql#L45-47"
    excerpt_sha256: "sha256:cf7d929384f1721f29a3975da337f3c599a7c3784ddc66dcd88ec11a5911e656"
---
run_status_transitions keeps trigger run_status_transitions_immutable: CREATE TRIGGER run_status_transitions_immutable BEFORE UPDATE OR DELETE ON run_status_transitions FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [run](/contexts/run)
