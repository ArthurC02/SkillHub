---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0056_creation_sessions.sql#L44-45"
    excerpt_sha256: "sha256:ff3915e4954cc3239bcfdf6229e422c5414764316e67516d25bd480926f54dac"
---
creation_session_events keeps trigger creation_event_immutable: CREATE TRIGGER creation_event_immutable BEFORE UPDATE ON creation_session_events FOR EACH ROW EXECUTE FUNCTION forbid_creation_event_update()

# Relations
- Contexts: [creation](/contexts/creation)
