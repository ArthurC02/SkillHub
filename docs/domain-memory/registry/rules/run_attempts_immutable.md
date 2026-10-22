---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0079_attempt_provider_unreachable.sql#L4-8"
    excerpt_sha256: "sha256:56a316e9723940e676dabb5b5c007b5edb4d3567f2efa54658299049bd35d7a3"
---
run_attempts keeps trigger run_attempts_immutable: CREATE TRIGGER run_attempts_immutable BEFORE UPDATE OR DELETE ON run_attempts FOR EACH ROW EXECUTE FUNCTION enforce_immutable( 'provider_run_id', 'error_class', 'error_message', 'started_at', 'finished_at', 'object_grants_expire_at', 'obje…

# Relations
- Contexts: [run](/contexts/run)
