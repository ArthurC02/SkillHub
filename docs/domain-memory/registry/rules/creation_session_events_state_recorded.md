---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0114_creation_events_record_state.sql#L2-3"
    excerpt_sha256: "sha256:89d665e894a43794f8c9e8ac6f5adce5f39f3cb3fe8f2abc356b7201984867a4"
---
creation_session_events keeps check creation_session_events_state_recorded: CONSTRAINT creation_session_events_state_recorded CHECK (state IS NOT NULL AND btrim(state) <> '') NOT VALID

# Relations
- Contexts: [creation](/contexts/creation)
